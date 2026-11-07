import SwiftUI
import WebKit

// The complete cybersecurity console stays available alongside native workflows.
// Its WebKit store is private and ephemeral; credentials never enter JavaScript.
@MainActor @Observable final class ConsoleSession: NSObject, WKNavigationDelegate, WKUIDelegate, WKDownloadDelegate {
    let webView: WKWebView
    let address: ServerAddress
    var loading = false
    var error: String?
    var requestedPath: String?
    private let expired: () -> Void
    private var downloads: [ObjectIdentifier: URL] = [:]
    private var activeDownloads: [ObjectIdentifier: WKDownload] = [:]

    init(address: ServerAddress, expired: @escaping () -> Void) {
        self.address = address; self.expired = expired
        let configuration = WKWebViewConfiguration()
        configuration.websiteDataStore = .nonPersistent()
        configuration.preferences.javaScriptCanOpenWindowsAutomatically = false
        webView = WKWebView(frame: .zero, configuration: configuration)
        super.init()
        webView.navigationDelegate = self; webView.uiDelegate = self
        webView.allowsBackForwardNavigationGestures = true
    }
    static func cookie(address: ServerAddress, credential: SessionCredential) throws -> HTTPCookie {
        // A bearer credential is opaque, but cookie/header control characters are
        // never valid. The server currently issues base64url JWT credentials.
        guard validBearerCredential(credential.value) else {
            throw ClientError.message("Invalid session credential.")
        }
        let lifetime = max(0, Int((credential.expires ?? Date().addingTimeInterval(3600)).timeIntervalSinceNow))
        let value = "afterglow_token=\(credential.value); Path=/; HttpOnly; SameSite=Lax; Max-Age=\(lifetime)" + (address.origin.scheme == "https" ? "; Secure" : "")
        guard let cookie = HTTPCookie.cookies(withResponseHeaderFields: ["Set-Cookie": value], for: address.origin).first,
              cookie.name == "afterglow_token", cookie.isHTTPOnly else { throw ClientError.message("Could not establish the console session.") }
        return cookie
    }
    func prepare(_ credential: SessionCredential) async throws {
        let cookie = try Self.cookie(address: address, credential: credential)
        await withCheckedContinuation { continuation in
            webView.configuration.websiteDataStore.httpCookieStore.setCookie(cookie) { continuation.resume() }
        }
    }
    func load(_ path: String) {
        guard requestedPath != path, let url = URL(string: path, relativeTo: address.origin)?.absoluteURL,
              address.contains(url), path.hasPrefix("/"), !path.hasPrefix("//"),
              URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.contains(where: { $0.name == "token" }) != true else { return }
        requestedPath = path; error = nil
        webView.load(URLRequest(url: url))
    }
    func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        guard let url = navigationAction.request.url else { decisionHandler(.cancel); return }
        #if DEBUG
        NSLog("APTEX action scheme=%@ download=%@", url.scheme ?? "unknown", String(navigationAction.shouldPerformDownload))
        #endif
        if url.scheme == "blob",
           let embedded=URL(string:String(url.absoluteString.dropFirst(5))), address.contains(embedded) {
            decisionHandler(.download)
        } else if navigationAction.shouldPerformDownload, url.scheme == "data", navigationAction.sourceFrame.securityOrigin.host == address.origin.host {
            decisionHandler(.download)
        } else if address.contains(url), url.user == nil, url.password == nil {
            decisionHandler(navigationAction.shouldPerformDownload ? .download : .allow)
        } else {
            if (navigationAction.navigationType == .linkActivated || navigationAction.sourceFrame.securityOrigin.host == address.origin.host), url.scheme == "https", url.user == nil, url.password == nil {
                UIApplication.shared.open(url)
            }
            decisionHandler(.cancel)
        }
    }
    func webView(_ webView: WKWebView, decidePolicyFor navigationResponse: WKNavigationResponse, decisionHandler: @escaping (WKNavigationResponsePolicy) -> Void) {
        guard let url = navigationResponse.response.url else { decisionHandler(.cancel);return }
        if url.scheme == "blob",let embedded=URL(string:String(url.absoluteString.dropFirst(5))),address.contains(embedded) {decisionHandler(.download);return}
        if url.scheme == "data" {decisionHandler(.download);return}
        guard address.contains(url) else {decisionHandler(.cancel);return}
        let disposition = (navigationResponse.response as? HTTPURLResponse)?.value(forHTTPHeaderField: "Content-Disposition") ?? ""
        decisionHandler(disposition.lowercased().contains("attachment") || !navigationResponse.canShowMIMEType ? .download : .allow)
    }
    func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration, for navigationAction: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
        guard let url = navigationAction.request.url else { return nil }
        #if DEBUG
        NSLog("APTEX new-window scheme=%@",url.scheme ?? "unknown")
        #endif
        if url.scheme == "blob",let embedded=URL(string:String(url.absoluteString.dropFirst(5))),address.contains(embedded) {
            webView.startDownload(using:navigationAction.request) { [weak self] download in
                guard let self else {return};download.delegate=self;self.activeDownloads[ObjectIdentifier(download)]=download
            }
        } else if address.contains(url) { webView.load(URLRequest(url: url)) }
        else if url.scheme == "https", url.user == nil, url.password == nil { UIApplication.shared.open(url) }
        return nil
    }
    func webView(_ webView: WKWebView, didStartProvisionalNavigation navigation: WKNavigation!) { loading = true; error = nil }
    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        loading = false
        if webView.url?.path == "/login" { expired() }
    }
    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
        loading = false
        if (error as? URLError)?.code != .cancelled { self.error = error.localizedDescription }
    }
    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) { loading = false; self.error = error.localizedDescription }
    func webViewWebContentProcessDidTerminate(_ webView: WKWebView) { loading = false; error = "Console was interrupted. Reload to reconnect." }
    private var presenter: UIViewController? {
        var current = webView.window?.rootViewController
        while let shown = current?.presentedViewController { current = shown }
        return current
    }
    func webView(_ webView: WKWebView, runJavaScriptAlertPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping () -> Void) {
        guard let presenter else { completionHandler(); return }
        let alert = UIAlertController(title: "APTEX", message: message, preferredStyle: .alert)
        alert.addAction(UIAlertAction(title: "OK", style: .default) { _ in completionHandler() }); presenter.present(alert, animated: true)
    }
    func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (Bool) -> Void) {
        guard let presenter else { completionHandler(false); return }
        let alert = UIAlertController(title: "APTEX", message: message, preferredStyle: .alert)
        alert.addAction(UIAlertAction(title: "Cancel", style: .cancel) { _ in completionHandler(false) })
        alert.addAction(UIAlertAction(title: "OK", style: .default) { _ in completionHandler(true) }); presenter.present(alert, animated: true)
    }
    func webView(_ webView: WKWebView, navigationAction: WKNavigationAction, didBecome download: WKDownload) { download.delegate = self; activeDownloads[ObjectIdentifier(download)] = download }
    func webView(_ webView: WKWebView, navigationResponse: WKNavigationResponse, didBecome download: WKDownload) { download.delegate = self; activeDownloads[ObjectIdentifier(download)] = download }
    func download(_ download: WKDownload, decideDestinationUsing response: URLResponse, suggestedFilename: String, completionHandler: @escaping (URL?) -> Void) {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent("APTEX-" + UUID().uuidString, isDirectory: true)
        do {
            try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true, attributes: [.protectionKey: FileProtectionType.complete])
            let safe = URL(fileURLWithPath: suggestedFilename).lastPathComponent
            let url = directory.appendingPathComponent(safe.isEmpty || safe == "." || safe == ".." ? "download" : safe)
            downloads[ObjectIdentifier(download)] = url; completionHandler(url)
        } catch { self.error = error.localizedDescription; completionHandler(nil) }
    }
    func download(_ download: WKDownload, willPerformHTTPRedirection response: HTTPURLResponse, newRequest request: URLRequest, decisionHandler: @escaping (WKDownload.RedirectPolicy) -> Void) {
        decisionHandler(request.url.map(address.contains) == true ? .allow : .cancel)
    }
    func webView(_ webView: WKWebView, runJavaScriptTextInputPanelWithPrompt prompt: String, defaultText: String?, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (String?) -> Void) {
        guard let presenter else { completionHandler(nil);return }
        let alert=UIAlertController(title:"APTEX",message:prompt,preferredStyle:.alert)
        alert.addTextField { $0.text=defaultText }
        alert.addAction(UIAlertAction(title:"Cancel",style:.cancel){_ in completionHandler(nil)})
        alert.addAction(UIAlertAction(title:"OK",style:.default){_ in completionHandler(alert.textFields?.first?.text)})
        presenter.present(alert,animated:true)
    }
    func downloadDidFinish(_ download: WKDownload) {
        activeDownloads.removeValue(forKey: ObjectIdentifier(download))
        #if DEBUG
        NSLog("APTEX download finished presenter=%@", String(presenter != nil))
        #endif
        guard let url = downloads.removeValue(forKey: ObjectIdentifier(download)) else { return }
        do {try FileManager.default.setAttributes([.protectionKey:FileProtectionType.complete],ofItemAtPath:url.path)}
        catch {self.error=error.localizedDescription;try? FileManager.default.removeItem(at:url.deletingLastPathComponent());return}
        guard let presenter else { try? FileManager.default.removeItem(at:url.deletingLastPathComponent());return }
        let share = UIActivityViewController(activityItems: [url], applicationActivities: nil)
        share.view.accessibilityIdentifier = "export-share-sheet"
        share.popoverPresentationController?.sourceView = webView
        share.popoverPresentationController?.sourceRect = CGRect(x: webView.bounds.midX, y: webView.bounds.midY, width: 1, height: 1)
        share.completionWithItemsHandler = { _,_,_,_ in try? FileManager.default.removeItem(at:url.deletingLastPathComponent()) }
        presenter.present(share, animated: true)
    }
    func download(_ download: WKDownload, didFailWithError error: Error, resumeData: Data?) {
        activeDownloads.removeValue(forKey: ObjectIdentifier(download))
        if let url = downloads.removeValue(forKey: ObjectIdentifier(download)) { try? FileManager.default.removeItem(at: url.deletingLastPathComponent()) }
        #if DEBUG
        NSLog("APTEX download error %@ %ld", (error as NSError).domain, (error as NSError).code)
        #endif
        self.error = error.localizedDescription
    }
}
struct ConsoleWebView: UIViewRepresentable {
    let session: ConsoleSession
    func makeUIView(context: Context) -> WKWebView { session.webView }
    func updateUIView(_ uiView: WKWebView, context: Context) {}
}
struct ConsoleView: View {
    @Environment(AppStore.self) private var store
    @State private var session: ConsoleSession?
    @State private var preparing = false
    @State private var error: String?
    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                if let error = error ?? session?.error { ErrorBanner(message: error).padding(8) }
                if let session {
                    if session.loading { ProgressView().frame(height: 3) }
                    ConsoleWebView(session: session)
                } else { ContentUnavailableView("Full cybersecurity console", systemImage: "terminal", description: Text("Graphs, workflows, traffic, interception, model settings, and advanced tools.")) }
            }.clientNavigationTitle("Console").navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItemGroup(placement: .topBarTrailing) {
                        Button { session?.webView.goBack() } label: { Image(systemName: "chevron.left") }.accessibilityLabel("Back").disabled(session?.webView.canGoBack != true)
                        Button { session?.webView.reload() } label: { Image(systemName: "arrow.clockwise") }.accessibilityLabel("Reload")
                        Menu {
                            Button("Tasks") { store.consolePath = "/function/tasks" }
                            Button("Findings") { store.consolePath = "/function/findings" }
                            Button("Models and settings") { store.consolePath = "/system/llm" }
                            Button("Workflows") { store.consolePath = "/function/workflows" }
                            Button("Traffic") { store.consolePath = "/function/traffic" }
                        } label: { Image(systemName: "square.grid.2x2") }
                    }
                }
                .task(id: store.selectedTab) { if store.selectedTab == 3 { await prepare() } }
                .onChange(of: store.consolePath) { _, path in session?.load(path) }
        }
    }
    private func prepare() async {
        if let session { session.load(store.consolePath); return }
        guard !preparing, let api = store.api, let credential = store.credential else { return }
        preparing = true; defer { preparing = false }
        let candidate = ConsoleSession(address: api.address) {
            Task {
                do { let _: SessionResponse = try await api.get("/api/v1/auth/session") }
                catch { store.handle(error, from: api) }
            }
        }
        do { try await candidate.prepare(credential); session = candidate; candidate.load(store.consolePath) }
        catch { self.error = error.localizedDescription }
    }
}
