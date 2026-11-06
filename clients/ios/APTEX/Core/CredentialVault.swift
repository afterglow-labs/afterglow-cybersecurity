// Adapted from afterglow-labs/odysseus-ios at f81c2cd09b90 (AGPL-3.0).
import Foundation
import Security

struct SessionCredential: Codable {
    let value: String
    let secure: Bool
    let expires: Date?
    var valid: Bool { expires.map { $0 > Date() } ?? true }
}

enum CredentialVault {
    private static func restoreDisabledKey(_ server: String) -> String {
        "aptex.session.restore-disabled." + Data(server.utf8).base64EncodedString()
    }
    private static func query(_ server: String) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword,
         kSecAttrService as String: "dev.afterglow.aptex.session",
         kSecAttrAccount as String: server]
    }

    static func load(server: String) -> SessionCredential? {
        // This flag contains no credential. It prevents restoring an old token
        // when signing entitlements prevented its replacement or deletion.
        guard !UserDefaults.standard.bool(forKey: restoreDisabledKey(server)) else { return nil }
        var request = query(server)
        request[kSecReturnData as String] = true
        request[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        guard SecItemCopyMatching(request as CFDictionary, &result) == errSecSuccess,
              let data = result as? Data,
              let credential = try? JSONDecoder().decode(SessionCredential.self, from: data), credential.valid else { return nil }
        return credential
    }

    static func save(_ credential: SessionCredential?, server: String) throws {
        UserDefaults.standard.set(true, forKey: restoreDisabledKey(server))
        let request = query(server)
        guard let credential else { SecItemDelete(request as CFDictionary); return }
        #if DEBUG
        if let status = ProcessInfo.processInfo.environment["APTEX_TEST_KEYCHAIN_STATUS"],
           status == "-34018" {
            throw ClientError.message("Could not securely save the session (Keychain -34018).")
        }
        #endif
        let data = try JSONEncoder().encode(credential)
        let attributes: [String: Any] = [kSecValueData as String: data,
                                       kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly]
        var status = SecItemUpdate(request as CFDictionary, attributes as CFDictionary)
        if status == errSecItemNotFound {
            status = SecItemAdd(request.merging(attributes) { _, new in new } as CFDictionary, nil)
        }
        guard status == errSecSuccess else { throw ClientError.message("Could not securely save the session (Keychain \(status)).") }
        UserDefaults.standard.removeObject(forKey: restoreDisabledKey(server))
    }
}

struct SessionCredentialStorage {
    let load: (String) -> SessionCredential?
    let save: (SessionCredential?, String) throws -> Void
    static let keychain = SessionCredentialStorage(load: { CredentialVault.load(server: $0) },
        save: { try CredentialVault.save($0, server: $1) })
}
