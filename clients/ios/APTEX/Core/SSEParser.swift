// Adapted from afterglow-labs/odysseus-ios at f81c2cd09b90 (AGPL-3.0).
// APTEX adds durable event IDs and a bound on the complete frame, including data lines.
import Foundation

struct SSEFrame: Equatable { let event: String; let id: String?; let data: String }
struct SSEParser {
    private var event = "message"
    private var id: String?
    private var lines: [String] = []
    private var size = 0
    mutating func consume(_ line: String) throws -> SSEFrame? {
        size += line.utf8.count
        guard size <= 4 * 1024 * 1024 else { throw ClientError.message("The server sent an oversized stream event.") }
        if line.isEmpty {
            defer { event = "message"; id = nil; lines = []; size = 0 }
            return lines.isEmpty ? nil : SSEFrame(event: event, id: id, data: lines.joined(separator: "\n"))
        }
        if line.hasPrefix(":") { return nil }
        let parts = line.split(separator: ":", maxSplits: 1, omittingEmptySubsequences: false)
        var value = parts.count > 1 ? String(parts[1]) : ""
        if value.hasPrefix(" ") { value.removeFirst() }
        if parts[0] == "event" { event = value }
        else if parts[0] == "id", !value.contains("\0") { id = value }
        else if parts[0] == "data" { lines.append(value) }
        return nil
    }
}

enum ClientError: LocalizedError {
    case message(String), http(Int, String), incompatibleServer
    var unauthorized: Bool { if case .http(401, _) = self { return true }; return false }
    var ambiguous: Bool { if case .http(let status, _) = self { return status >= 500 }; return true }
    var errorDescription: String? {
        switch self {
        case .message(let text): return text
        case .http(let code, let text): return "\(text) (HTTP \(code))"
        case .incompatibleServer: return "This is not a compatible APTEX server. Use your desktop's APTEX address, including its port."
        }
    }
}
