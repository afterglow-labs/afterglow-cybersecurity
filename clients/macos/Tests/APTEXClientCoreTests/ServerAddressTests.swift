import XCTest
@testable import APTEXClientCore

final class ServerAddressTests: XCTestCase {
    func testDesktopAddressesAndNavigationBoundary() throws {
        for value in ["https://desktop.example:8787", "http://192.168.1.20:8787", "http://100.100.1.2:8787", "http://desktop.local:8787", "http://localhost:8787", "http://[::1]:8787", "http://[fd00::1]:8787"] {
            let server = try ServerAddress(value)
            XCTAssertTrue(server.contains(server.home))
            XCTAssertFalse(server.contains(URL(string: "https://unrelated.example/")!))
        }
        let server = try ServerAddress("https://desktop.example")
        XCTAssertTrue(server.contains(URL(string: "https://desktop.example:443/report?id=2")!))
        XCTAssertFalse(server.contains(URL(string: "http://desktop.example/")!))
        XCTAssertFalse(server.contains(URL(string: "https://desktop.example:8443/")!))
        XCTAssertEqual(server.home.path, "/function/tasks")
    }
    func testRejectsCredentialLeaksAndExecutableSchemes() {
        for value in ["", "javascript:alert(1)", "file:///etc/passwd", "https://user:secret@desktop.example", "https://desktop.example/path", "https://desktop.example?token=secret", "https://desktop.example#fragment", "https://desktop.example:0", "https://desktop.example:99999", "http://example.com", "http://8.8.8.8", "http://0177.0.0.1", "https://desktop.example\\@other.example", "https://desktop.example\n/path"] {
            XCTAssertThrowsError(try ServerAddress(value), value)
        }
    }
}
