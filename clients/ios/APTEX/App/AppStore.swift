import Foundation
import Observation

@MainActor @Observable final class AppStore {
    var api: APTEXAPI?
    var metadata: ServerMetadata?
    var username = UserDefaults.standard.string(forKey: "aptex.username") ?? "Afterglow"
    var serverText = UserDefaults.standard.string(forKey: "aptex.server") ?? ""
    var busy = false
    var error: String?
    var consolePath = "/function/tasks"
    var selectedTab = 0
    let storage: SessionCredentialStorage
    init(storage: SessionCredentialStorage = .keychain) {
        self.storage = storage
        #if DEBUG
        if ProcessInfo.processInfo.arguments.contains("--uitesting-reset"),
           let raw = ProcessInfo.processInfo.environment["APTEX_UI_TEST_SERVER"],
           let address = try? ServerAddress(raw), address.origin.host == "127.0.0.1", address.origin.port == 18768 {
            serverText = address.origin.absoluteString; username = "Afterglow"
            try? storage.save(nil, serverText)
            for (key, value) in ["aptex.language": "en", "aptex.theme": "Obsidian", "aptex.appearance": "dark"] { UserDefaults.standard.set(value, forKey: key) }
        }
        #endif
    }
    var credential: SessionCredential?
    var connectionID = UUID()
    var drafts: [Int64: ChatSession] = [:]

    func restore() async {
        guard !busy, api == nil, !serverText.isEmpty else { return }
        busy = true; error = nil
        defer { busy = false }
        do {
            let address = try ServerAddress(serverText)
            guard let saved = storage.load(address.origin.absoluteString) else { return }
            let candidate = APTEXAPI(address: address, token: saved.value)
            let meta = try await candidate.verifyServer()
            let response: SessionResponse = try await candidate.get("/api/v1/auth/session")
            username = response.user.username
            accept(candidate, meta: meta, credential: saved)
        } catch {
            handle(error)
        }
    }
    func login(password: String) async {
        guard !busy else { return }
        busy = true; error = nil
        defer { busy = false }
        do {
            let address = try ServerAddress(serverText)
            let candidate = APTEXAPI(address: address)
            let response = try await candidate.login(username: username, password: password)
            guard let expiry = Date.sessionExpiry(response.expires_at), expiry > Date(), validBearerCredential(response.token) else {
                throw ClientError.message("The server returned an invalid session.")
            }
            let authenticated = APTEXAPI(address: address, token: response.token)
            let saved = SessionCredential(value: response.token, secure: address.origin.scheme == "https", expires: expiry)
            do { try storage.save(saved, address.origin.absoluteString) }
            catch {
                let _: EmptyResponse? = try? await authenticated.send("/api/v1/auth/logout", body: [String: String]())
                throw error
            }
            PendingInputVault.clear(server: address.origin.absoluteString)
            username = response.username
            let meta = try await authenticated.verifyServer()
            accept(authenticated, meta: meta, credential: saved)
        } catch { handle(error, invalidateSession: false) }
    }
    private func accept(_ api: APTEXAPI, meta: ServerMetadata, credential: SessionCredential) {
        self.api = api; metadata = meta; self.credential = credential
        serverText = api.address.origin.absoluteString
        UserDefaults.standard.set(serverText, forKey: "aptex.server")
        UserDefaults.standard.set(username, forKey: "aptex.username")
        connectionID = UUID(); error = nil
    }
    func logout() async {
        guard let api, !busy else { return }
        busy = true; error = nil
        defer { busy = false }
        do {
            let _: EmptyResponse = try await api.send("/api/v1/auth/logout", body: [String: String]())
            try storage.save(nil, api.address.origin.absoluteString)
            PendingInputVault.clear(server: api.address.origin.absoluteString)
            clearSession()
        } catch { handle(error) }
    }
    func switchServer() { clearSession(); error = nil }
    func handle(_ failure: Error, from source: APTEXAPI? = nil, invalidateSession: Bool = true) {
        if let source, api !== source { return }
        if failure is CancellationError || (failure as? URLError)?.code == .cancelled { return }
        if invalidateSession, let client = failure as? ClientError, client.unauthorized {
            if let origin = api?.address.origin.absoluteString ?? (try? ServerAddress(serverText).origin.absoluteString) {
                try? storage.save(nil, origin)
                PendingInputVault.clear(server: origin)
            }
            clearSession()
        }
        error = failure.localizedDescription
    }
    private func clearSession() {
        for draft in drafts.values { draft.closeClientSession() }
        drafts = [:]; api = nil; metadata = nil; credential = nil
        connectionID = UUID(); selectedTab = 0; consolePath = "/function/tasks"
    }
    func openConsole(_ path: String) { consolePath = path; selectedTab = 3 }
    func chat(_ conversation: Conversation) -> ChatSession {
        if let existing = drafts[conversation.id] { return existing }
        let model = ChatSession(conversation: conversation, store: self)
        drafts[conversation.id] = model
        return model
    }
}
