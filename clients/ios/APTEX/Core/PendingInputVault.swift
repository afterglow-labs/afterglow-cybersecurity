import Foundation
import Security

// Preserve an unconfirmed admission across process termination. The message and
// attachments stay in this app's Keychain, never preferences or web storage.
enum PendingInputVault {
    private static let service = "com.afterglowlabs.aptex.pending-input"
    private static func epochKey(_ server: String) -> String { "aptex.pending.epoch." + Data(server.utf8).base64EncodedString() }
    private static func account(_ server: String, _ conversation: Int64) -> String {
        server + "|" + (UserDefaults.standard.string(forKey: epochKey(server)) ?? "initial") + "|" + String(conversation)
    }
    private static func query(_ account: String) -> [String: Any] {
        [kSecClass as String:kSecClassGenericPassword, kSecAttrService as String:service, kSecAttrAccount as String:account]
    }
    private static func blocked(_ account: String) -> String { "aptex.pending.blocked." + Data(account.utf8).base64EncodedString() }
    static func load(server: String, conversation: Int64) -> PendingInput? {
        let name=account(server,conversation)
        guard !UserDefaults.standard.bool(forKey:blocked(name)) else { return nil }
        var request=query(name); request[kSecReturnData as String]=true;request[kSecMatchLimit as String]=kSecMatchLimitOne
        var value: CFTypeRef?
        guard SecItemCopyMatching(request as CFDictionary,&value)==errSecSuccess,let data=value as? Data else { return nil }
        return try? JSONDecoder().decode(PendingInput.self,from:data)
    }
    static func save(_ input: PendingInput?, server: String, conversation: Int64) throws {
        let name=account(server,conversation), request=query(name)
        UserDefaults.standard.set(true,forKey:blocked(name))
        guard let input else { SecItemDelete(request as CFDictionary);return }
        let values: [String:Any]=[kSecValueData as String:try JSONEncoder().encode(input),kSecAttrAccessible as String:kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly]
        var status=SecItemUpdate(request as CFDictionary,values as CFDictionary)
        if status==errSecItemNotFound {status=SecItemAdd(request.merging(values){_,new in new} as CFDictionary,nil)}
        guard status==errSecSuccess else {throw ClientError.message("Could not securely save the pending message (Keychain \(status)).")}
        UserDefaults.standard.removeObject(forKey:blocked(name))
    }
    static func clear(server: String) {
        // Rotate even if entitlement/storage failures prevent deletion. Old
        // admissions cannot reappear in a later account session.
        UserDefaults.standard.set(UUID().uuidString,forKey:epochKey(server))
        let request: [String:Any]=[kSecClass as String:kSecClassGenericPassword,kSecAttrService as String:service,kSecReturnAttributes as String:true,kSecMatchLimit as String:kSecMatchLimitAll]
        var value: CFTypeRef?
        if SecItemCopyMatching(request as CFDictionary,&value)==errSecSuccess,let records=value as? [[String:Any]] {
            for record in records {
                if let name=record[kSecAttrAccount as String] as? String,name.hasPrefix(server+"|") {SecItemDelete(query(name) as CFDictionary)}
            }
        }
    }
}
