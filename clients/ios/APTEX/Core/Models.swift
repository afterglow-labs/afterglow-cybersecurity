import Foundation

struct ServerMetadata: Decodable {
    let service: String
    let api_version: String
    let server_version: String
}
struct LoginResponse: Decodable { let token: String; let username: String; let expires_at: String }
struct SessionUser: Decodable { let username: String }
struct SessionResponse: Decodable { let user: SessionUser; let expires_at: String }
struct EmptyResponse: Decodable {}
struct TaskList: Decodable { let tasks: [SecurityTask]; let active: String? }
struct SeverityCounts: Decodable {
    let critical: Int; let high: Int; let medium: Int; let low: Int
    var total: Int { critical + high + medium + low }
}
struct SecurityTask: Decodable, Identifiable, Hashable {
    let id: String; let name: String?; let description: String; let goal: String; let status: String
    let goals_total: Int?; let goals_met: Int?; let created_at: String?
    let llm_profile_id: Int64?
    var title: String { name.flatMap { $0.isEmpty ? nil : $0 } ?? description }
}
struct AgentList: Decodable { let agents: [AgentSummary] }
struct AgentSummary: Decodable, Identifiable {
    let id: String; let key: String; let name: String; let enabled: Bool?; let role: String?
}
struct ProfileList: Decodable { let profiles: [ModelProfile] }
struct ModelProfile: Decodable, Identifiable {
    let id: String; let name: String; let model: String; let auth_mode: String?
    let connected: Bool?; let access_program_cyber: String?; let is_default: Bool?
    var label: String { name.isEmpty ? model : name + " · " + model }
    var available: Bool { auth_mode != "chatgpt_plan" || connected == true }
}
struct ConversationList: Decodable { let conversations: [Conversation] }
struct Conversation: Decodable, Identifiable, Hashable {
    let id: Int64; let agent_key: String; let title: String; let llm_profile_id: Int64?; let running: Bool?
}
struct MessagePage: Decodable {
    let items: [Activity]; let cursor: Int64; let running: Bool; let hasMore: Bool
}
struct Activity: Decodable, Identifiable, Hashable {
    let seq: Int64; let worker: String?; let kind: String?; let summary: String?; let detail: String?
    let tool: String?; let is_error: Bool?; let ts: String?
    var id: Int64 { seq }
    var body: String { (detail?.isEmpty == false ? detail : summary) ?? "" }
    var isUser: Bool { kind == "user" || worker == "user" }
    var isMessage: Bool { isUser || kind == "text" || kind == "result" }
    func withDetail(_ value: String) -> Activity {
        Activity(seq:seq,worker:worker,kind:kind,summary:summary,detail:value,tool:tool,is_error:is_error,ts:ts)
    }
}
struct ChatRun: Decodable { let id: String; let status: String; let active: Bool }
struct ChatPrompt: Codable { let text: String; let attachments: [ChatAttachment] }
struct ChatInput: Decodable, Identifiable {
    let id: String; let status: String; let prompt: ChatPrompt; let revision: String?
}
struct ChatControls: Decodable {
    let revision: String; let history_revision: String; let paused: Bool; let context_changing: Bool
    let active: ChatRun?; let inputs: [ChatInput]
    var running: Bool { active?.active == true }
}
struct ConversationState: Decodable {
    let cursor: String; let running: Bool; let paused: Bool
    let history_revision: String; let control_revision: String; let context_changing: Bool
}
struct ChatAttachment: Codable, Identifiable, Hashable {
    let name: String; let path: String; let size: Int64; let abs: String?
    var id: String { path }
}
struct UploadResponse: Decodable { let attachments: [ChatAttachment] }
struct PendingInput: Codable {
    let client_request_id: String
    let mode: String
    let message: String
    let attachments: [ChatAttachment]
    init(message: String, attachments: [ChatAttachment], queued: Bool) {
        client_request_id = UUID().uuidString
        mode = queued ? "queue" : "send"
        self.message = message; self.attachments = attachments
    }
}
struct FindingPage: Decodable { let items: [Finding]; let total: Int }
struct Finding: Decodable, Identifiable, Hashable {
    let id: String; let name: String?; let vulnclass: String; let severity: String; let status: String
    let summary: String; let evidence: String; let report: String?; let task_id: String?
    var title: String { name.flatMap { $0.isEmpty ? nil : $0 } ?? vulnclass }
}
struct GoalList: Decodable { let goals: [TaskGoal] }
struct TaskGoal: Decodable, Identifiable { let id: String; let text: String; let state: String }
struct ConstraintList: Decodable { let constraints: [TaskConstraint] }
struct TaskConstraint: Decodable, Identifiable { let id: String; let kind: String; let text: String }

struct NewConversation: Encodable { let agent_key: String; let title: String; let llm_profile_id: Int64? }
struct NewTask: Encodable {
    let name: String; let description: String; let goal: String
    let llm_profile_id: Int64?; let timeout_seconds: Int
}
struct RevisionAction: Encodable { let expected_revision: String }
struct TaskAction: Encodable { let action: String }
struct ProfileSelection: Encodable { let llm_profile_id: Int64? }
struct Credentials: Encodable { let username: String; let password: String }

extension Date {
    static func sessionExpiry(_ value: String) -> Date? {
        let parser = ISO8601DateFormatter()
        if let date = parser.date(from: value) { return date }
        parser.formatOptions.insert(.withFractionalSeconds)
        return parser.date(from: value)
    }
}

func selectedProfileID(_ selection: String) throws -> Int64? {
    if selection.isEmpty { return nil }
    guard let id=Int64(selection),id>0 else {throw ClientError.message("The selected model profile has an invalid identifier.")}
    return id
}

func validBearerCredential(_ token: String) -> Bool {
    let allowed=CharacterSet(charactersIn:"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~+/=")
    return !token.isEmpty && token.utf8.count<=4096 && token.unicodeScalars.allSatisfy(allowed.contains)
}

struct ActivityDetail: Decodable { let detail: String }
