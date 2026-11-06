import AppKit
import WebKit
import APTEXClientCore

@main @MainActor
struct APTEXMain {
    static func main() {
        let app = NSApplication.shared
        let delegate = ClientApplication()
        app.setActivationPolicy(.regular)
        app.delegate = delegate
        withExtendedLifetime(delegate) { app.run() }
    }
}

@MainActor
final class ClientApplication: NSObject, NSApplicationDelegate, WKNavigationDelegate, WKUIDelegate, WKDownloadDelegate {
    private var window: NSWindow!
    private var webView: WKWebView!
    private var addressField: NSTextField!
    private var connectPanel: NSView!
    private var address: ServerAddress?
    private var pendingDownloads: [WKDownload] = []
    private let preference = "desktopServerOrigin"
    private var smoke = false

    func applicationDidFinishLaunching(_ notification: Notification) {
        makeMenu()
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1180, height: 800), styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
        window.title = "APTEX by Afterglow Labs"
        window.minSize = NSSize(width: 760, height: 540)
        window.backgroundColor = NSColor(srgbRed: 0.035, green: 0.055, blue: 0.075, alpha: 1)
        window.appearance = NSAppearance(named: .darkAqua)
        window.setFrameAutosaveName("APTEXClientWindow")
        let config = WKWebViewConfiguration()
        config.websiteDataStore = .default()
        webView = WKWebView(frame: .zero, configuration: config)
        webView.navigationDelegate = self
        webView.uiDelegate = self
        webView.allowsBackForwardNavigationGestures = true
        webView.translatesAutoresizingMaskIntoConstraints = false
        window.contentView!.addSubview(webView)
        NSLayoutConstraint.activate([
            webView.leadingAnchor.constraint(equalTo: window.contentView!.leadingAnchor),
            webView.trailingAnchor.constraint(equalTo: window.contentView!.trailingAnchor),
            webView.topAnchor.constraint(equalTo: window.contentView!.topAnchor),
            webView.bottomAnchor.constraint(equalTo: window.contentView!.bottomAnchor)
        ])
        makeConnectionPanel()
        window.center()
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        let args = ProcessInfo.processInfo.arguments
        if let flag = args.firstIndex(of: "--smoke-test"), args.indices.contains(flag + 1) {
            smoke = true
            do { load(try ServerAddress(args[flag + 1])) } catch { fputs("Invalid smoke-test address\n", stderr); exit(2) }
            DispatchQueue.main.asyncAfter(deadline: .now() + 45) { fputs("WebKit client smoke test timed out\n", stderr); exit(3) }
        } else if let saved = UserDefaults.standard.string(forKey: preference), let server = try? ServerAddress(saved) {
            load(server)
        } else { showServer() }
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }

    private func makeMenu() {
        let bar = NSMenu()
        let appItem = NSMenuItem(); let appMenu = NSMenu()
        appMenu.addItem(withTitle: "About APTEX", action: #selector(about), keyEquivalent: "")
        appMenu.addItem(withTitle: "Server…", action: #selector(showServer), keyEquivalent: ",")
        appMenu.addItem(.separator())
        appMenu.addItem(withTitle: "Quit APTEX", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        appItem.submenu = appMenu; bar.addItem(appItem)
        let editItem = NSMenuItem(); editItem.title = "Edit"; let edit = NSMenu(title: "Edit")
        for (title, action, key) in [("Undo", "undo:", "z"), ("Cut", "cut:", "x"), ("Copy", "copy:", "c"), ("Paste", "paste:", "v"), ("Select All", "selectAll:", "a")] {
            edit.addItem(withTitle: title, action: Selector(action), keyEquivalent: key)
        }
        editItem.submenu = edit; bar.addItem(editItem)
        let viewItem = NSMenuItem(); viewItem.title = "View"; let view = NSMenu(title: "View")
        view.addItem(withTitle: "Reload", action: #selector(reload), keyEquivalent: "r")
        view.addItem(withTitle: "Back", action: #selector(back), keyEquivalent: "[")
        view.addItem(withTitle: "Forward", action: #selector(forward), keyEquivalent: "]")
        view.addItem(withTitle: "Open in Browser", action: #selector(openInBrowser), keyEquivalent: "")
        view.addItem(withTitle: "Zoom In", action: #selector(zoomIn), keyEquivalent: "+")
        view.addItem(withTitle: "Zoom Out", action: #selector(zoomOut), keyEquivalent: "-")
        viewItem.submenu = view; bar.addItem(viewItem)
        let connectionItem = NSMenuItem(); connectionItem.title = "Connection"; let connection = NSMenu(title: "Connection")
        connection.addItem(withTitle: "Server…", action: #selector(showServer), keyEquivalent: "")
        connection.addItem(withTitle: "Disconnect", action: #selector(disconnect), keyEquivalent: "")
        connectionItem.submenu = connection; bar.addItem(connectionItem)
        for menu in [appMenu, view, connection] { for item in menu.items where item.action != #selector(NSApplication.terminate(_:)) { item.target = self } }
        NSApp.mainMenu = bar
    }

    private func makeConnectionPanel() {
        connectPanel = NSView(); connectPanel.translatesAutoresizingMaskIntoConstraints = false
        window.contentView!.addSubview(connectPanel)
        NSLayoutConstraint.activate([
            connectPanel.leadingAnchor.constraint(equalTo: window.contentView!.leadingAnchor),
            connectPanel.trailingAnchor.constraint(equalTo: window.contentView!.trailingAnchor),
            connectPanel.topAnchor.constraint(equalTo: window.contentView!.topAnchor),
            connectPanel.bottomAnchor.constraint(equalTo: window.contentView!.bottomAnchor)
        ])
        let brand = NSTextField(labelWithString: "APTEX")
        brand.font = .monospacedSystemFont(ofSize: 32, weight: .semibold)
        let publisher = NSTextField(labelWithString: "by Afterglow Labs")
        publisher.textColor = .secondaryLabelColor
        let detail = NSTextField(wrappingLabelWithString: "Connect to APTEX running on your desktop.")
        detail.textColor = .secondaryLabelColor
        let label = NSTextField(labelWithString: "Desktop server address")
        label.font = .systemFont(ofSize: 13, weight: .medium)
        addressField = NSTextField()
        addressField.placeholderString = "https://your-desktop:8787"
        addressField.setAccessibilityLabel("Desktop server address")
        addressField.target = self; addressField.action = #selector(connect)
        let button = NSButton(title: "Connect", target: self, action: #selector(connect))
        button.bezelStyle = .rounded; button.keyEquivalent = "\r"
        let stack = NSStackView(views: [brand, publisher, detail, label, addressField, button])
        stack.orientation = .vertical; stack.alignment = .leading; stack.spacing = 14
        stack.translatesAutoresizingMaskIntoConstraints = false
        connectPanel.addSubview(stack)
        NSLayoutConstraint.activate([
            stack.centerXAnchor.constraint(equalTo: connectPanel.centerXAnchor),
            stack.centerYAnchor.constraint(equalTo: connectPanel.centerYAnchor),
            stack.widthAnchor.constraint(equalToConstant: 440),
            addressField.widthAnchor.constraint(equalTo: stack.widthAnchor)
        ])
    }

    @objc private func showServer() {
        webView.isHidden = true; connectPanel.isHidden = false
        addressField.stringValue = address?.origin.absoluteString ?? UserDefaults.standard.string(forKey: preference) ?? ""
        window.makeFirstResponder(addressField)
    }
    @objc private func connect() {
        do { load(try ServerAddress(addressField.stringValue)) }
        catch { showError(error.localizedDescription) }
    }
    private func load(_ server: ServerAddress) {
        address = server
        if !smoke { UserDefaults.standard.set(server.origin.absoluteString, forKey: preference) }
        connectPanel.isHidden = true; webView.isHidden = false
        window.subtitle = server.origin.host ?? ""
        webView.load(URLRequest(url: server.home))
    }
    @objc private func disconnect() {
        webView.stopLoading(); address = nil
        webView.loadHTMLString("", baseURL: nil)
        UserDefaults.standard.removeObject(forKey: preference)
        window.subtitle = ""; showServer()
    }
    @objc private func reload() { if address != nil { webView.reload() } }
    @objc private func back() { webView.goBack() }
    @objc private func forward() { webView.goForward() }
    @objc private func zoomIn() { webView.pageZoom = min(2, webView.pageZoom + 0.1) }
    @objc private func zoomOut() { webView.pageZoom = max(0.6, webView.pageZoom - 0.1) }
    @objc private func openInBrowser() { if let url = webView.url, address?.contains(url) == true { NSWorkspace.shared.open(url) } }
    @objc private func about() {
        NSApp.orderFrontStandardAboutPanel(options: [.applicationName: "APTEX by Afterglow Labs", .applicationVersion: "1.0.0", .credits: NSAttributedString(string: "Desktop client for your APTEX server. AGPL-3.0.")])
    }
    private func showError(_ message: String) {
        let alert = NSAlert(); alert.messageText = "APTEX"; alert.informativeText = message; alert.addButton(withTitle: "OK")
        alert.beginSheetModal(for: window)
    }

    func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        guard let url = navigationAction.request.url else { decisionHandler(.cancel); return }
        if address == nil && url.absoluteString == "about:blank" { decisionHandler(.allow); return }
        let blob = address.map { url.absoluteString.hasPrefix("blob:" + $0.origin.absoluteString + "/") } ?? false
        if address?.contains(url) == true || blob {
            decisionHandler(navigationAction.shouldPerformDownload ? .download : .allow)
        } else {
            if navigationAction.navigationType == .linkActivated || navigationAction.targetFrame?.isMainFrame == true {
                if url.scheme == "https" { NSWorkspace.shared.open(url) }
            }
            decisionHandler(.cancel)
        }
    }
    func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration, for navigationAction: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
        if let url = navigationAction.request.url {
            if address?.contains(url) == true { webView.load(navigationAction.request) }
            else if url.scheme == "https" { NSWorkspace.shared.open(url) }
        }
        return nil
    }
    func webView(_ webView: WKWebView, decidePolicyFor navigationResponse: WKNavigationResponse, decisionHandler: @escaping (WKNavigationResponsePolicy) -> Void) {
        decisionHandler(navigationResponse.canShowMIMEType ? .allow : .download)
    }
    func webView(_ webView: WKWebView, navigationAction: WKNavigationAction, didBecome download: WKDownload) { track(download) }
    func webView(_ webView: WKWebView, navigationResponse: WKNavigationResponse, didBecome download: WKDownload) { track(download) }
    private func track(_ download: WKDownload) { pendingDownloads.append(download); download.delegate = self }
    func download(_ download: WKDownload, decideDestinationUsing response: URLResponse, suggestedFilename: String, completionHandler: @escaping (URL?) -> Void) {
        let panel = NSSavePanel(); panel.nameFieldStringValue = (suggestedFilename as NSString).lastPathComponent
        panel.beginSheetModal(for: window) { result in completionHandler(result == .OK ? panel.url : nil) }
    }
    func downloadDidFinish(_ download: WKDownload) { pendingDownloads.removeAll { $0 === download } }
    func download(_ download: WKDownload, didFailWithError error: Error, resumeData: Data?) {
        pendingDownloads.removeAll { $0 === download }; showError(error.localizedDescription)
    }
    func webView(_ webView: WKWebView, runOpenPanelWith parameters: WKOpenPanelParameters, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping ([URL]?) -> Void) {
        guard let url = frame.request.url, address?.contains(url) == true else { completionHandler(nil); return }
        let panel = NSOpenPanel(); panel.allowsMultipleSelection = parameters.allowsMultipleSelection
        panel.canChooseDirectories = parameters.allowsDirectories
        panel.beginSheetModal(for: window) { result in completionHandler(result == .OK ? panel.urls : nil) }
    }
    func webView(_ webView: WKWebView, runJavaScriptAlertPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping () -> Void) {
        let alert = NSAlert(); alert.messageText = "APTEX"; alert.informativeText = String(message.prefix(8192)); alert.addButton(withTitle: "OK")
        alert.beginSheetModal(for: window) { _ in completionHandler() }
    }
    func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (Bool) -> Void) {
        let alert = NSAlert(); alert.messageText = "APTEX"; alert.informativeText = String(message.prefix(8192)); alert.addButton(withTitle: "OK"); alert.addButton(withTitle: "Cancel")
        alert.beginSheetModal(for: window) { result in completionHandler(result == .alertFirstButtonReturn) }
    }
    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) { navigationError(error) }
    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) { navigationError(error) }
    private func navigationError(_ error: Error) {
        guard (error as NSError).code != NSURLErrorCancelled else { return }
        if smoke { fputs("WebKit navigation failed: \(error.localizedDescription)\n", stderr); exit(4) }
        showServer(); showError("Could not connect to the desktop server. \(error.localizedDescription)")
    }
    func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
        if smoke { exit(5) }
        showServer(); showError("The web view stopped. Reconnect to your desktop server.")
    }
    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        if smoke { verifySmoke(attempt: 0) }
    }
    private func verifySmoke(attempt: Int) {
        webView.evaluateJavaScript("document.querySelector('#password') !== null || document.documentElement.hasAttribute('data-aptex-smoke-ready')") { [weak self] result, error in
            guard let self else { exit(6) }
            if result as? Bool != true {
                if attempt >= 60 { fputs("The APTEX page did not render\n", stderr); exit(7) }
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.25) { self.verifySmoke(attempt: attempt + 1) }; return
            }
            self.webView.takeSnapshot(with: nil) { image, error in
                guard let image, let data = image.tiffRepresentation, let bitmap = NSBitmapImageRep(data: data), let png = bitmap.representation(using: .png, properties: [:]) else { exit(8) }
                if let destination = ProcessInfo.processInfo.environment["APTEX_SMOKE_SCREENSHOT"] { try? png.write(to: URL(fileURLWithPath: destination)) }
                print("PASS: native APTEX client loaded and rendered its server page")
                exit(0)
            }
        }
    }
}
