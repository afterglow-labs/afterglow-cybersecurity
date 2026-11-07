// Exercise the actual iOS networking sources on a Mac against an owned,
// disposable Go backend. Read synthetic fixture credentials only from stdin.
import Foundation

private struct FixtureConfig: Decodable { let origin: String; let username: String; let password: String }
private enum ObservedState: Error { case complete }
@main private struct ContractCheck {
    static func main() async throws {
        let config=try JSONDecoder().decode(FixtureConfig.self,from:FileHandle.standardInput.readDataToEndOfFile())
        let connection=APTEXAPI(address:try ServerAddress(config.origin))
        let metadata=try await connection.verifyServer()
        guard metadata.server_version=="platform-candidate" else {throw ClientError.message("This check requires the disposable platform-candidate fixture; refusing a live server.")}
        let login=try await connection.login(username:config.username,password:config.password)
        let api=APTEXAPI(address:connection.address,token:login.token)
        let tasks:TaskList=try await api.get("/api/v1/tasks")
        let _:ProfileList=try await api.get("/api/v1/llm/profiles")
        let agents:AgentList=try await api.get("/api/v1/agents")
        guard let agent=agents.agents.first(where:{$0.key=="mainagent"}) ?? agents.agents.first else {throw ClientError.message("Fixture has no agent.")}
        let conversation:Conversation=try await api.send("/api/v1/conversations",body:NewConversation(agent_key:agent.key,title:"Native contract fixture",llm_profile_id:nil))
        let _:ConversationList=try await api.get("/api/v1/conversations")
        let _:MessagePage=try await api.get("/api/v1/conversations/\(conversation.id)/messages")
        let _:ActivityDetail=try await api.get("/api/v1/conversations/\(conversation.id)/messages/1")
        let _:ChatControls=try await api.get("/api/v1/conversations/\(conversation.id)/controls")
        let _:FindingPage=try await api.get("/api/v1/exploration/findings?page=1&limit=100")
        let file=FileManager.default.temporaryDirectory.appendingPathComponent("aptex-contract-"+UUID().uuidString+".txt")
        try Data("Synthetic native client contract evidence".utf8).write(to:file)
        defer {try? FileManager.default.removeItem(at:file)}
        let attachments=try await api.upload(file:file,conversation:conversation.id)
        guard !attachments.isEmpty else {throw ClientError.message("Fixture upload did not return metadata.")}
        do {
            try await api.events(conversation:conversation.id,cursor:0) { frame in
                if frame.event=="state" {
                    _=try JSONDecoder().decode(ConversationState.self,from:Data(frame.data.utf8))
                    throw ObservedState.complete
                }
            }
            throw ClientError.message("The real backend did not send a conversation state.")
        } catch ObservedState.complete {}
        let _:EmptyResponse=try await api.send("/api/v1/auth/logout",body:[String:String]())
        do {let _:SessionResponse=try await api.get("/api/v1/auth/session");throw ClientError.message("Revoked session remained valid.")}
        catch let error as ClientError {guard error.unauthorized else {throw error}}
        print("Native networking contract passed: metadata, identity, login, tasks (\(tasks.tasks.count)), agents, model profiles, conversations, history, controls, findings, attachment upload, SSE state and logout revocation.")
    }
}
