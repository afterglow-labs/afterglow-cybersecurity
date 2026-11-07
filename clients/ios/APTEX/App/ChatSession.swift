import Foundation
import Observation

@MainActor @Observable final class ChatSession {
    let conversation: Conversation
    private weak var store: AppStore?
    private let connectedAPI: APTEXAPI?
    var activities: [Activity] = []
    var controls: ChatControls?
    var draft = ""
    var profileID: Int64?
    var uploadName = ""
    private var fullDetails: [Int64:String] = [:]
    private struct DetailRequest:Hashable {let seq:Int64;let revision:String?}
    private var loadingDetails: Set<DetailRequest> = []
    private var uploadTask: Task<Void,Never>?
    var attachments: [ChatAttachment] = []
    var pending: PendingInput?
    var loading = false
    var sending = false
    var uploading = false
    var hasMore = false
    var running = false
    var streamConnected = false
    var error: String?
    private(set) var cursor: Int64 = 0
    private var observer: Task<Void, Never>?
    private var refreshID = UUID()
    init(conversation: Conversation, store: AppStore) {
        self.conversation = conversation; self.store = store; connectedAPI = store.api; profileID=conversation.llm_profile_id
        if let server=store.api?.address.origin.absoluteString, let saved=PendingInputVault.load(server:server,conversation:conversation.id) {
            pending=saved;draft=saved.message;attachments=saved.attachments
        }
    }
    private var api: APTEXAPI? { store?.api === connectedAPI ? connectedAPI : nil }
    private var base: String { "/api/v1/conversations/\(conversation.id)" }
    func stopObserving() { observer?.cancel(); observer = nil; streamConnected = false }
    func closeClientSession() { stopObserving();uploadTask?.cancel() }
    func startObserving() async {
        guard observer == nil, api != nil else { return }
        await refresh()
        guard !Task.isCancelled, api != nil else { return }
        observer = Task { [weak self] in
            var attempt = 0
            while !Task.isCancelled {
                guard let self, let api = self.api else { return }
                do {
                    try await api.events(conversation: self.conversation.id, cursor: self.cursor) { [weak self] frame in
                        guard let self else { return }
                        try await self.consume(frame)
                    }
                    attempt = min(attempt + 1, 5)
                } catch is CancellationError { return }
                catch let failure as URLError where failure.code == .cancelled { return }
                catch {
                    if let client = error as? ClientError, client.unauthorized { self.store?.handle(client, from: api); return }
                    self.streamConnected = false
                    self.error = "Connection interrupted. Reconnecting…"
                    attempt = min(attempt + 1, 5)
                }
                do { try await Task.sleep(for: .seconds(min(15, 1 << attempt))) } catch { return }
                guard !Task.isCancelled else { return }
                // Refresh handles history changes that happened while the phone was offline.
                await self.refresh()
            }
        }
    }
    private func consume(_ frame: SSEFrame) async throws {
        try Task.checkCancellation()
        guard let data = frame.data.data(using: .utf8) else { throw ClientError.message("Invalid event data.") }
        switch frame.event {
        case "activity":
            let item = try JSONDecoder().decode(Activity.self, from: data)
            guard frame.id == String(item.seq) else { throw ClientError.message("Invalid event cursor.") }
            if !activities.contains(where: { $0.id == item.id }) { activities.append(item); activities.sort { $0.seq < $1.seq } }
            cursor = max(cursor, item.seq); streamConnected = true; error = nil
        case "state":
            let state = try JSONDecoder().decode(ConversationState.self, from: data)
            streamConnected = true; running = state.running; error = nil
            if controls?.history_revision != state.history_revision { await refresh() }
            else if controls?.revision != state.control_revision, let api {
                controls = try await api.get(base + "/controls")
            }
        case "heartbeat": streamConnected = true
        case "session_expired": throw ClientError.http(401, "Your session ended. Sign in again.")
        case "unavailable": throw ClientError.http(503, "Conversation temporarily unavailable.")
        default: break
        }
    }
    func refresh() async {
        guard let api else { return }
        let requestID = UUID(); refreshID = requestID
        loading = activities.isEmpty
        defer { if refreshID == requestID { loading = false } }
        do {
            var before: ChatControls = try await api.get(base + "/controls")
            var page: MessagePage = try await api.get(base + "/messages")
            var after: ChatControls = try await api.get(base + "/controls")
            for _ in 0..<2 where before.history_revision != after.history_revision {
                before = after; page = try await api.get(base + "/messages"); after = try await api.get(base + "/controls")
            }
            guard refreshID == requestID else { return }
            // If history keeps changing, reconnect will retry; don't publish a mixed snapshot.
            guard before.history_revision == after.history_revision else { throw ClientError.message("History is changing. Reconnect to refresh it.") }
            if controls?.history_revision != after.history_revision { fullDetails = [:] }
            activities = page.items.map { item in fullDetails[item.id].map(item.withDetail) ?? item }; cursor = max(cursor, page.cursor)
            hasMore = page.hasMore; controls = after; running = page.running || after.running; error = nil
        } catch { fail(error) }
    }
    func loadEarlier() async {
        guard let api, hasMore, let first = activities.first, !loading else { return }
        loading = true; defer { loading = false }
        let revision = controls?.history_revision
        do {
            let page: MessagePage = try await api.get(base + "/messages?before=\(first.seq)")
            guard controls?.history_revision == revision else { return }
            var byID = Dictionary(uniqueKeysWithValues: activities.map { ($0.id, $0) })
            for item in page.items { byID[item.id] = item }
            activities = byID.values.sorted { $0.seq < $1.seq }; hasMore = page.hasMore
        } catch { fail(error) }
    }
    func send() async {
        guard let api, !sending, !uploading else { return }
        guard pending != nil || !draft.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || !attachments.isEmpty else { return }
        if pending == nil {
            let input=PendingInput(message:draft,attachments:attachments,queued:running)
            do {try PendingInputVault.save(input,server:api.address.origin.absoluteString,conversation:conversation.id);pending=input}
            catch {fail(error);return}
        }
        guard let outgoing = pending else { return }
        sending = true; error = nil; defer { sending = false }
        do {
            let _: ChatInput = try await api.send(base + "/controls/inputs", body: outgoing)
            guard self.api === api else {return}
            // Same request ID/body survives ambiguous failures. Acknowledgement alone clears it.
            try? PendingInputVault.save(nil,server:api.address.origin.absoluteString,conversation:conversation.id)
            pending = nil; draft = ""; attachments = []
            await refresh()
        } catch {
            guard self.api === api else {return}
            if let client = error as? ClientError, !client.ambiguous {
                try? PendingInputVault.save(nil,server:api.address.origin.absoluteString,conversation:conversation.id)
                pending = nil
            }
            fail(error)
        }
    }
    func control() async {
        guard let api, let controls, !sending else { return }
        sending = true; defer { sending = false }
        do {
            let _: EmptyResponse = try await api.send(base + "/controls/" + (controls.paused ? "resume" : "pause"), body: RevisionAction(expected_revision: controls.revision))
            await refresh()
        } catch { await refresh(); fail(error) }
    }
    func selectProfile(_ id: String) async -> Bool {
        guard let api, !running, !sending, pending == nil else { return false }
        sending=true;defer {sending=false}
        do {
            let _: EmptyResponse = try await api.send(base + "/profile", method: "PATCH", body: ProfileSelection(llm_profile_id: try selectedProfileID(id)))
            profileID=try selectedProfileID(id)
            return true
        } catch { fail(error); return false }
    }
    func startUpload(_ files: [URL], removeImportedCopies: Bool = false) {
        guard uploadTask == nil else {
            if removeImportedCopies {for file in files {try? FileManager.default.removeItem(at:file.deletingLastPathComponent())}}
            return
        }
        uploadTask=Task {
            defer {
                uploadTask=nil
                if removeImportedCopies {
                    for file in files {try? FileManager.default.removeItem(at:file.deletingLastPathComponent())}
                }
            }
            await upload(files)
        }
    }
    func cancelUpload() { uploadTask?.cancel() }
    func upload(_ files: [URL]) async {
        guard let api, !uploading, pending == nil else { return }
        uploading = true; defer { uploading = false;uploadName="" }
        do {
            for file in files {
                try Task.checkCancellation();uploadName=file.lastPathComponent
                attachments.append(contentsOf: try await api.upload(file: file, conversation: conversation.id))
            }
        } catch { fail(error) }
    }
    func expand(_ item: Activity) async {
        guard let api, fullDetails[item.id] == nil else { return }
        let request=DetailRequest(seq:item.id,revision:controls?.history_revision)
        guard loadingDetails.insert(request).inserted else {return}
        defer {loadingDetails.remove(request)}
        let revision=controls?.history_revision
        do {
            let full: ActivityDetail = try await api.get(base + "/messages/\(item.seq)")
            guard revision == controls?.history_revision else {return}
            fullDetails[item.id]=full.detail
            if let index = activities.firstIndex(where: { $0.id == item.id }) { activities[index] = activities[index].withDetail(full.detail) }
        } catch { fail(error) }
    }
    private func fail(_ failure: Error) {
        if failure is CancellationError || (failure as? URLError)?.code == .cancelled { return }
        error = failure.localizedDescription
        if let client = failure as? ClientError, client.unauthorized { store?.handle(client, from: connectedAPI) }
    }
}
