import Foundation

// All redirects stay on the selected origin. A redirect never forwards a bearer
// credential, password, or uploaded evidence to another server or a weaker scheme.
final class OriginRedirectPolicy: NSObject, URLSessionTaskDelegate {
    let address: ServerAddress
    init(_ address: ServerAddress) { self.address = address }
    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) {
        completionHandler(request.url.map(address.contains) == true ? request : nil)
    }
}

final class APTEXAPI {
    let address: ServerAddress
    let token: String?
    private let session: URLSession
    init(address: ServerAddress, token: String? = nil, configuration: URLSessionConfiguration = .ephemeral) {
        self.address = address; self.token = token
        configuration.httpShouldSetCookies = false
        configuration.httpCookieStorage = nil
        configuration.urlCache = nil
        configuration.requestCachePolicy = .reloadIgnoringLocalCacheData
        configuration.timeoutIntervalForRequest = 20
        configuration.timeoutIntervalForResource = 3600
        session = URLSession(configuration: configuration, delegate: OriginRedirectPolicy(address), delegateQueue: nil)
    }
    deinit { session.invalidateAndCancel() }

    func url(_ path: String) throws -> URL {
        guard path.hasPrefix("/api/"), !path.contains(".."), let url = URL(string: path, relativeTo: address.origin)?.absoluteURL,
              address.contains(url), URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.contains(where: { $0.name == "token" }) != true
        else { throw ClientError.message("Invalid API address.") }
        return url
    }
    func request(_ path: String, method: String = "GET", body: Data? = nil, contentType: String = "application/json") throws -> URLRequest {
        var request = URLRequest(url: try url(path))
        request.httpMethod = method; request.httpBody = body
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue(UserDefaults.standard.string(forKey: "aptex.language") ?? "en", forHTTPHeaderField: "Accept-Language")
        if body != nil { request.setValue(contentType, forHTTPHeaderField: "Content-Type") }
        if let token {
            guard validBearerCredential(token) else {throw ClientError.message("Invalid session credential.")}
            request.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
        }
        request.httpShouldHandleCookies = false
        return request
    }
    private func validate(_ response: URLResponse, data: Data = Data()) throws {
        guard let http = response as? HTTPURLResponse else { throw ClientError.message("The server returned an invalid response.") }
        guard (200...299).contains(http.statusCode) else {
            let message = (try? JSONDecoder().decode([String: String].self, from: data))?["error"] ?? HTTPURLResponse.localizedString(forStatusCode: http.statusCode)
            throw ClientError.http(http.statusCode, String(message.prefix(2000)))
        }
    }
    func get<T: Decodable>(_ path: String, as type: T.Type = T.self) async throws -> T {
        try await perform(try request(path), as: type)
    }
    func send<T: Decodable, B: Encodable>(_ path: String, method: String = "POST", body: B, as type: T.Type = T.self) async throws -> T {
        try await perform(try request(path, method: method, body: JSONEncoder().encode(body)), as: type)
    }
    func perform<T: Decodable>(_ request: URLRequest, as type: T.Type) async throws -> T {
        let (bytes, response) = try await session.bytes(for: request)
        defer { bytes.task.cancel() }
        var data = Data()
        let limit = (response as? HTTPURLResponse).map { (200...299).contains($0.statusCode) } == true ? 32 * 1024 * 1024 : 64 * 1024
        for try await byte in bytes {
            guard data.count < limit else { throw ClientError.message("The server response is too large.") }
            data.append(byte)
        }
        try validate(response, data: data)
        return try JSONDecoder().decode(type, from: data)
    }
    func verifyServer() async throws -> ServerMetadata {
        let metadata: ServerMetadata
        var bootstrap=try request("/api/v1/meta")
        bootstrap.setValue(nil,forHTTPHeaderField:"Authorization")
        do { metadata = try await perform(bootstrap,as:ServerMetadata.self) }
        catch let error as ClientError {
            if case .http(404, _) = error { throw ClientError.incompatibleServer }
            throw error
        } catch is DecodingError { throw ClientError.incompatibleServer }
        guard metadata.service == "aptex", metadata.api_version == "1" else { throw ClientError.incompatibleServer }
        return metadata
    }
    func login(username: String, password: String) async throws -> LoginResponse {
        // Identity check precedes transmission of the password, including for callers
        // other than the connection screen.
        _ = try await verifyServer()
        return try await send("/api/v1/auth/login", body: Credentials(username: username, password: password))
    }
    func events(conversation: Int64, cursor: Int64, receive: @escaping (SSEFrame) async throws -> Void) async throws {
        var request = try request("/api/v1/conversations/\(conversation)/events")
        request.setValue("text/event-stream", forHTTPHeaderField: "Accept")
        request.setValue(String(cursor), forHTTPHeaderField: "Last-Event-ID")
        let (bytes, response) = try await session.bytes(for: request)
        defer { bytes.task.cancel() }
        try validate(response)
        guard (response as? HTTPURLResponse)?.mimeType == "text/event-stream" else { throw ClientError.message("The server did not open an event stream.") }
        var parser = SSEParser()
        var line = Data()
        for try await byte in bytes {
            try Task.checkCancellation()
            if byte == 10 {
                if line.last == 13 { line.removeLast() }
                guard let text = String(data: line, encoding: .utf8) else { throw ClientError.message("The server sent invalid UTF-8.") }
                line.removeAll(keepingCapacity: true)
                if let frame = try parser.consume(text) { try await receive(frame) }
            } else {
                guard line.count < 4 * 1024 * 1024 else { throw ClientError.message("The server sent an oversized stream line.") }
                line.append(byte)
            }
        }
    }
    func upload(file: URL, conversation: Int64) async throws -> [ChatAttachment] {
        guard file.isFileURL else {throw ClientError.message("Choose a local file to upload.")}
        let access = file.startAccessingSecurityScopedResource()
        defer { if access { file.stopAccessingSecurityScopedResource() } }
        let size = try file.resourceValues(forKeys: [.fileSizeKey]).fileSize ?? 0
        guard size <= 32 * 1024 * 1024 else { throw ClientError.message("Choose a file smaller than 32 MB, or upload it in Console.") }
        let content = try Data(contentsOf: file, options: .mappedIfSafe)
        guard content.count <= 32 * 1024 * 1024 else { throw ClientError.message("Choose a file smaller than 32 MB, or upload it in Console.") }
        let boundary = "APTEX-" + UUID().uuidString
        let filename = file.lastPathComponent.replacingOccurrences(of: "\\", with: "_").replacingOccurrences(of: "\"", with: "_").replacingOccurrences(of: "\r", with: "_").replacingOccurrences(of: "\n", with: "_")
        var body = Data("--\(boundary)\r\nContent-Disposition: form-data; name=\"file\"; filename=\"\(filename)\"\r\nContent-Type: application/octet-stream\r\n\r\n".utf8)
        body.append(content); body.append(Data("\r\n--\(boundary)--\r\n".utf8))
        let result: UploadResponse = try await perform(try request("/api/v1/chat/upload?scope=session&id=conv-\(conversation)", method: "POST", body: body, contentType: "multipart/form-data; boundary=\(boundary)"), as: UploadResponse.self)
        return result.attachments
    }
}
