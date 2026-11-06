// Shared policy with the APTEX Mac client; keep origin validation consistent.
import Foundation

public struct ServerAddress: Equatable {
    public let origin: URL
    public var home: URL { origin.appendingPathComponent("function/tasks") }

    public init(_ input: String) throws {
        let raw = input.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !raw.isEmpty, !raw.unicodeScalars.contains(where: { CharacterSet.whitespacesAndNewlines.contains($0) || CharacterSet.controlCharacters.contains($0) }),
              var parts = URLComponents(string: raw.contains("://") ? raw : "https://" + raw),
              let scheme = parts.scheme?.lowercased(), ["https", "http"].contains(scheme),
              let host = parts.host, !host.isEmpty, parts.user == nil, parts.password == nil,
              parts.query == nil, parts.fragment == nil, ["", "/"].contains(parts.path),
              parts.port == nil || (1...65535).contains(parts.port!),
              !raw.contains("\\") else { throw AddressError.invalid }
        if scheme == "http" && !Self.isPrivateHost(host) { throw AddressError.publicHTTP }
        parts.scheme = scheme
        parts.host = host.lowercased()
        parts.path = ""
        guard let url = parts.url else { throw AddressError.invalid }
        origin = url
    }

    public func contains(_ url: URL) -> Bool {
        guard let other = try? ServerAddress(Self.originString(url)) else { return false }
        return other.origin.scheme == origin.scheme && other.origin.host?.lowercased() == origin.host?.lowercased()
            && Self.port(other.origin) == Self.port(origin)
    }
    private static func port(_ url: URL) -> Int { url.port ?? (url.scheme == "https" ? 443 : 80) }
    private static func originString(_ url: URL) -> String {
        guard var components = URLComponents(url: url, resolvingAgainstBaseURL: false) else { return "" }
        components.path = ""; components.query = nil; components.fragment = nil
        return components.string ?? ""
    }
    private static func isPrivateHost(_ input: String) -> Bool {
        let host = input.lowercased().trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        if host == "localhost" || host.hasSuffix(".localhost") || host.hasSuffix(".local") || !host.contains(".") && !host.contains(":") { return true }
        if host.contains(":") { return host == "::1" || host.hasPrefix("fc") || host.hasPrefix("fd") || ["fe8", "fe9", "fea", "feb"].contains(where: host.hasPrefix) }
        let fields = host.split(separator: ".", omittingEmptySubsequences: false)
        guard fields.count == 4 else { return false }
        let bytes = fields.compactMap { UInt8($0) }
        guard bytes.count == 4, zip(fields, bytes).allSatisfy({ String($0.1) == $0.0 }) else { return false }
        return bytes[0] == 10 || bytes[0] == 127 || bytes[0] == 192 && bytes[1] == 168
            || bytes[0] == 172 && (16...31).contains(bytes[1])
            || bytes[0] == 100 && (64...127).contains(bytes[1])
            || bytes[0] == 169 && bytes[1] == 254
    }
}

public enum AddressError: LocalizedError {
    case invalid, publicHTTP
    public var errorDescription: String? {
        switch self {
        case .invalid: return "Enter the desktop server's address, without a path, password, query, or fragment."
        case .publicHTTP: return "Use HTTPS for a public address, or use your desktop's private network or VPN address."
        }
    }
}
