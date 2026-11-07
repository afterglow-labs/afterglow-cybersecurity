import XCTest
@testable import APTEX

final class MockProtocol: URLProtocol {
    static let lock = NSLock()
    static var handler: ((URLRequest) throws -> (Int, Data))?
    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func startLoading() {
        Self.lock.lock(); let handler = Self.handler; Self.lock.unlock()
        do {
            guard let handler else { throw URLError(.badServerResponse) }
            let (status, data) = try handler(request)
            let response = HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil, headerFields: ["Content-Type":"application/json"])!
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data); client?.urlProtocolDidFinishLoading(self)
        } catch { client?.urlProtocol(self, didFailWithError: error) }
    }
    override func stopLoading() {}
}

final class ClientTests: XCTestCase {
    override func tearDown() { MockProtocol.lock.lock(); MockProtocol.handler = nil; MockProtocol.lock.unlock() }
    func mockAPI(token: String? = nil, handler: @escaping (URLRequest) throws -> (Int, Data)) throws -> APTEXAPI {
        MockProtocol.lock.lock(); MockProtocol.handler = handler; MockProtocol.lock.unlock()
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [MockProtocol.self]
        return APTEXAPI(address: try ServerAddress("https://desktop.example:8787"), token: token, configuration: configuration)
    }
    func testOriginsAndPrivateTransport() throws {
        for value in ["https://desktop.example:8787", "http://localhost:18768", "http://100.90.2.3:8787", "http://192.168.1.8:8787", "http://[fd00::1]:8787"] { XCTAssertNoThrow(try ServerAddress(value)) }
        for value in ["http://example.com", "https://user:secret@example.com", "https://example.com/chat", "https://example.com?token=x", "https://example.com/#fragment", "file:///tmp/x", "https://example.com\\@attacker.example"] { XCTAssertThrowsError(try ServerAddress(value)) }
        let address = try ServerAddress("https://DESKTOP.example:443/")
        XCTAssertEqual(address, try ServerAddress("https://desktop.example"))
        XCTAssertTrue(address.contains(URL(string: "https://desktop.example/api/v1/meta")!))
        for target in ["http://desktop.example/api/v1/meta", "https://desktop.example:8787/api/v1/meta", "https://attacker.example", "https://user:pass@desktop.example"] { XCTAssertFalse(address.contains(URL(string: target)!)) }
    }
    func testIdentityIsCheckedBeforePasswordTransmission() async throws {
        var paths: [String] = []
        let api = try mockAPI(token:"fixture-saved-session") { request in
            XCTAssertNil(request.value(forHTTPHeaderField:"Authorization"))
            paths.append(request.url!.path)
            return (200, Data(#"{"service":"odysseus","api_version":"1","server_version":"fixture"}"#.utf8))
        }
        do { _ = try await api.login(username: "Afterglow", password: "fixture-only"); XCTFail("Sent credentials to another application") }
        catch ClientError.incompatibleServer {}
        XCTAssertEqual(paths, ["/api/v1/meta"])
    }
    func testBearerTransportRejectsURLCredentialsAndForeignOrigins() throws {
        let api = try mockAPI(token: "fixture.session") { _ in (200, Data("{}".utf8)) }
        let request = try api.request("/api/v1/tasks")
        XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer fixture.session")
        XCTAssertFalse(request.httpShouldHandleCookies)
        XCTAssertFalse(request.url!.absoluteString.contains("fixture.session"))
        XCTAssertFalse(validBearerCredential("bad\r\nheader"));XCTAssertFalse(validBearerCredential(String(repeating:"a",count:4097)))
        for path in ["https://attacker.example/api/v1/tasks", "//attacker.example/api/v1/tasks", "/api/v1/tasks?token=x", "/api/v1/../tasks"] { XCTAssertThrowsError(try api.url(path)) }
    }
    func testRedirectsCannotForwardCredentialsToAnotherOrigin() throws {
        let address = try ServerAddress("https://desktop.example:8787")
        let policy = OriginRedirectPolicy(address)
        let session = URLSession(configuration: .ephemeral)
        let task = session.dataTask(with: address.origin)
        let response = HTTPURLResponse(url: address.origin,statusCode:307,httpVersion:nil,headerFields:nil)!
        for target in ["https://attacker.example/api/v1/auth/login", "http://desktop.example:8787/api/v1/auth/login"] {
            policy.urlSession(session,task:task,willPerformHTTPRedirection:response,newRequest:URLRequest(url:URL(string:target)!)) { XCTAssertNil($0) }
        }
        policy.urlSession(session,task:task,willPerformHTTPRedirection:response,newRequest:URLRequest(url:address.origin.appendingPathComponent("api/v1/auth/session"))) { XCTAssertNotNil($0) }
        session.invalidateAndCancel()
    }
    @MainActor func testConsoleCookieIsHttpOnlyAndPrivate() throws {
        let address = try ServerAddress("https://desktop.example:8787")
        let credential = SessionCredential(value:"fixture.session",secure:true,expires:Date().addingTimeInterval(3600))
        let cookie = try ConsoleSession.cookie(address:address,credential:credential)
        XCTAssertTrue(cookie.isHTTPOnly); XCTAssertTrue(cookie.isSecure)
        XCTAssertEqual(cookie.path,"/"); XCTAssertEqual(cookie.domain,"desktop.example")
        let console = ConsoleSession(address:address,expired:{})
        XCTAssertFalse(console.webView.configuration.websiteDataStore.isPersistent)
        XCTAssertTrue(console.webView.configuration.userContentController.userScripts.isEmpty)
        XCTAssertThrowsError(try ConsoleSession.cookie(address:address,credential:SessionCredential(value:"unsafe; Path=/",secure:true,expires:nil)))
    }
    func testIntegerIdentifiersNeverPassThroughFloatingPoint() throws {
        let id: Int64 = 9_007_199_254_741_011
        let conversation = try JSONDecoder().decode(Conversation.self, from: Data("{\"id\":\(id),\"agent_key\":\"mainagent\",\"title\":\"fixture\"}".utf8))
        XCTAssertEqual(conversation.id,id)
        XCTAssertEqual(try selectedProfileID(String(id)),id)
        XCTAssertNil(try selectedProfileID(""))
        XCTAssertThrowsError(try selectedProfileID("9223372036854775808"))
        XCTAssertThrowsError(try selectedProfileID("not-a-profile"))
        let encoded = try JSONEncoder().encode(NewConversation(agent_key:"mainagent",title:"fixture",llm_profile_id:id))
        let object = try XCTUnwrap(JSONSerialization.jsonObject(with:encoded) as? [String:Any])
        XCTAssertEqual((object["llm_profile_id"] as? NSNumber)?.stringValue,String(id))
    }
    @MainActor func testEnglishAndSelectedLocaleUseTheirOwnResources() {
        XCTAssertEqual(clientLocalizedText("Settings",language:"en"),"Settings")
        XCTAssertEqual(clientLocalizedText("Settings",language:"es"),"Ajustes")
        XCTAssertEqual(clientLocalizedText("Settings",language:"ko"),"설정")
        XCTAssertEqual(clientLocalizedText("Settings",language:"unsupported"),"Settings")
    }
    func testFullDetailUsesTheRealDetailOnlyPayload() throws {
        let detail=try JSONDecoder().decode(ActivityDetail.self,from:Data(#"{"detail":"Full response beyond the summary"}"#.utf8))
        let activity=try JSONDecoder().decode(Activity.self,from:Data(#"{"seq":5,"kind":"user","worker":"mainagent","summary":"Short summary"}"#.utf8))
        let full=activity.withDetail(detail.detail)
        XCTAssertEqual(full.seq,5);XCTAssertEqual(full.kind,"user");XCTAssertTrue(full.isUser)
        XCTAssertEqual(full.body,"Full response beyond the summary")
    }
    @MainActor func testCancelledPhotoUploadRemovesItsPrivateCopyWithoutSending() async throws {
        let api=try mockAPI(token:"fixture.session") {_ in XCTFail("Cancelled upload reached the network");throw URLError(.cancelled)}
        let store=AppStore(storage:SessionCredentialStorage(load:{_ in nil},save:{_,_ in}));store.api=api
        let conversation=try JSONDecoder().decode(Conversation.self,from:Data(#"{"id":7,"agent_key":"mainagent","title":"fixture"}"#.utf8))
        let model=ChatSession(conversation:conversation,store:store)
        let directory=FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString,isDirectory:true)
        defer {try? FileManager.default.removeItem(at:directory)}
        try FileManager.default.createDirectory(at:directory,withIntermediateDirectories:true)
        let file=directory.appendingPathComponent("photo.png");try Data("synthetic image".utf8).write(to:file)
        model.startUpload([file],removeImportedCopies:true);model.cancelUpload()
        for _ in 0..<100 where FileManager.default.fileExists(atPath:directory.path) {try await Task.sleep(for:.milliseconds(10))}
        XCTAssertFalse(FileManager.default.fileExists(atPath:directory.path))
        XCTAssertFalse(model.uploading);XCTAssertTrue(model.attachments.isEmpty);XCTAssertNil(model.error)
    }
    @MainActor func testAnOldSendAcknowledgementCannotEraseTheNewSessionsPendingInput() async throws {
        let received=expectation(description:"Old admission is in flight")
        let release=DispatchSemaphore(value:0)
        let api=try mockAPI(token:"fixture.old-session") {request in
            XCTAssertTrue(request.url!.path.hasSuffix("/controls/inputs"));received.fulfill()
            guard release.wait(timeout:.now()+5) == .success else {throw URLError(.timedOut)}
            return (202,Data(#"{"id":"old-input","status":"completed","prompt":{"text":"old draft","attachments":[]}}"#.utf8))
        }
        let store=AppStore(storage:SessionCredentialStorage(load:{_ in nil},save:{_,_ in}));store.api=api
        let conversation=try JSONDecoder().decode(Conversation.self,from:Data(#"{"id":17,"agent_key":"mainagent","title":"fixture"}"#.utf8))
        let server=api.address.origin.absoluteString
        defer {PendingInputVault.clear(server:server)}
        let old=ChatSession(conversation:conversation,store:store);old.draft="old draft"
        let send=Task {await old.send()}
        await fulfillment(of:[received],timeout:3)
        // Logout and sign in again at the same origin while the old HTTP reply
        // is delayed. The new login's recovery record belongs to its new epoch.
        PendingInputVault.clear(server:server)
        store.api=APTEXAPI(address:api.address,token:"fixture.new-session")
        let current=PendingInput(message:"new unconfirmed draft",attachments:[],queued:false)
        try PendingInputVault.save(current,server:server,conversation:conversation.id)
        release.signal();await send.value
        XCTAssertEqual(PendingInputVault.load(server:server,conversation:conversation.id)?.client_request_id,current.client_request_id)
    }
    func testSSEMultilineIDsAndFrameBounds() throws {
        var parser = SSEParser()
        XCTAssertNil(try parser.consume(": heartbeat"))
        XCTAssertNil(try parser.consume("id: 9007199254741011"))
        XCTAssertNil(try parser.consume("event: activity"))
        XCTAssertNil(try parser.consume("data: first")); XCTAssertNil(try parser.consume("data: second"))
        XCTAssertEqual(try parser.consume(""),SSEFrame(event:"activity",id:"9007199254741011",data:"first\nsecond"))
        for _ in 0..<4 { _ = try parser.consume("data: " + String(repeating:"a",count:1_000_000)) }
        XCTAssertThrowsError(try parser.consume("data: " + String(repeating:"a",count:1_000_000)))
    }
    @MainActor func testAmbiguousSendRetriesTheSameAdmissionWithoutLosingTheDraft() async throws {
        var attempts = 0
        var ids: [String] = []
        let api = try mockAPI(token:"fixture.session") { request in
            let path = request.url!.path
            if path.hasSuffix("/controls/inputs") {
                attempts += 1
                // URLSession may expose the encoded body through an InputStream.
                var body = request.httpBody ?? Data()
                if body.isEmpty, let stream = request.httpBodyStream {
                    stream.open(); defer { stream.close() }
                    let buffer=UnsafeMutablePointer<UInt8>.allocate(capacity:4096);defer {buffer.deallocate()}
                    while stream.hasBytesAvailable { let n=stream.read(buffer,maxLength:4096);if n<=0 {break};body.append(buffer,count:n) }
                }
                let object = try XCTUnwrap(JSONSerialization.jsonObject(with:body) as? [String:Any])
                let id = try XCTUnwrap(object["client_request_id"] as? String); ids.append(id)
                if attempts == 1 { throw URLError(.networkConnectionLost) }
                return (202,Data("{\"id\":\"\(id)\",\"status\":\"queued\",\"prompt\":{\"text\":\"preserved draft\",\"attachments\":[]}}".utf8))
            }
            if path.hasSuffix("/controls") { return (200,Data(#"{"revision":"1","history_revision":"1","paused":false,"context_changing":false,"active":null,"inputs":[]}"#.utf8)) }
            return (200,Data(#"{"items":[],"cursor":0,"running":false,"hasMore":false}"#.utf8))
        }
        let store = AppStore(storage:SessionCredentialStorage(load:{_ in nil},save:{_,_ in}))
        store.api=api
        let conversation=try JSONDecoder().decode(Conversation.self,from:Data(#"{"id":7,"agent_key":"mainagent","title":"fixture"}"#.utf8))
        let model=ChatSession(conversation:conversation,store:store);model.draft="preserved draft"
        await model.send()
        XCTAssertNotNil(model.pending);XCTAssertEqual(model.draft,"preserved draft")
        let recovered=ChatSession(conversation:conversation,store:store)
        XCTAssertEqual(recovered.pending?.client_request_id,model.pending?.client_request_id)
        XCTAssertEqual(recovered.draft,"preserved draft")
        await recovered.send()
        XCTAssertEqual(ids.count,2);XCTAssertEqual(ids.first,ids.last)
        XCTAssertNil(recovered.pending);XCTAssertEqual(recovered.draft,"")
        XCTAssertNil(PendingInputVault.load(server:api.address.origin.absoluteString,conversation:conversation.id))
    }
    @MainActor func testWrongPasswordDoesNotEraseAnExistingSavedLogin() async throws {
        var touched=false
        let saved=SessionCredential(value:"saved.fixture",secure:false,expires:Date().addingTimeInterval(3600))
        let store=AppStore(storage:SessionCredentialStorage(load:{_ in saved},save:{_,_ in touched=true}))
        store.serverText="http://127.0.0.1:18768";store.username="Afterglow"
        await store.login(password:"wrong-fixture-password")
        XCTAssertFalse(touched);XCTAssertNil(store.api);XCTAssertNotNil(store.error)
    }
    @MainActor func testOldServerResponseCannotInvalidateTheNewConnection() throws {
        let old=APTEXAPI(address:try ServerAddress("https://old.example"),token:"fixture-old")
        let current=APTEXAPI(address:try ServerAddress("https://new.example"),token:"fixture-new")
        let store=AppStore(storage:SessionCredentialStorage(load:{_ in nil},save:{_,_ in XCTFail("Removed the current credential for an old response")}))
        store.api=current
        store.handle(ClientError.http(401,"Old session revoked"),from:old)
        XCTAssertTrue(store.api === current);XCTAssertNil(store.error)
    }
    func testPendingMessageClearingIsScopedToItsServer() throws {
        let first="https://pending-"+UUID().uuidString+".invalid",second="https://pending-"+UUID().uuidString+".invalid"
        defer {PendingInputVault.clear(server:first);PendingInputVault.clear(server:second)}
        let input=PendingInput(message:"synthetic pending input",attachments:[],queued:false)
        try PendingInputVault.save(input,server:first,conversation:7)
        try PendingInputVault.save(input,server:second,conversation:7)
        PendingInputVault.clear(server:first)
        XCTAssertNil(PendingInputVault.load(server:first,conversation:7))
        XCTAssertEqual(PendingInputVault.load(server:second,conversation:7)?.client_request_id,input.client_request_id)
    }
    func testKeychainIsScopedToTheServerAndDeletionPreventsRestore() throws {
        let first="https://fixture-"+UUID().uuidString+".invalid"
        let second="https://fixture-"+UUID().uuidString+".invalid"
        defer {try? CredentialVault.save(nil,server:first);try? CredentialVault.save(nil,server:second)}
        try CredentialVault.save(SessionCredential(value:"fixture-one",secure:true,expires:Date().addingTimeInterval(3600)),server:first)
        try CredentialVault.save(SessionCredential(value:"fixture-two",secure:true,expires:Date().addingTimeInterval(3600)),server:second)
        XCTAssertEqual(CredentialVault.load(server:first)?.value,"fixture-one")
        XCTAssertEqual(CredentialVault.load(server:second)?.value,"fixture-two")
        try CredentialVault.save(nil,server:first)
        XCTAssertNil(CredentialVault.load(server:first))
        XCTAssertEqual(CredentialVault.load(server:second)?.value,"fixture-two")
    }
    func testRealURLSessionLoginUploadAndRevocationAgainstOwnedFixture() async throws {
        let api = APTEXAPI(address:try ServerAddress("http://127.0.0.1:18768"))
        let login = try await api.login(username:"Afterglow",password:"fixture-aptex-1234")
        let signed = APTEXAPI(address:api.address,token:login.token)
        let session: SessionResponse = try await signed.get("/api/v1/auth/session")
        XCTAssertEqual(session.user.username,"Afterglow")
        let file=FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString+".txt")
        try Data("synthetic attachment".utf8).write(to:file);defer {try? FileManager.default.removeItem(at:file)}
        let attachments=try await signed.upload(file:file,conversation:7)
        XCTAssertEqual(attachments.first?.name,"fixture.txt")
        let _: EmptyResponse=try await signed.send("/api/v1/auth/logout",body:[String:String]())
        do {let _: SessionResponse=try await signed.get("/api/v1/auth/session");XCTFail("Revoked session was accepted")}
        catch let error as ClientError { XCTAssertTrue(error.unauthorized) }
    }
}
