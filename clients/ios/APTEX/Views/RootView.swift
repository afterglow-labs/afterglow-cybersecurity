import SwiftUI

struct RootView: View {
    @Environment(AppStore.self) private var store
    var body: some View {
        Group {
            if store.api != nil { WorkspaceView().id(store.connectionID) }
            else { ConnectionView() }
        }
    }
}
struct ConnectionView: View {
    @Environment(AppStore.self) private var store
    @Environment(\.colorScheme) private var colorScheme
    @State private var password = ""
    @FocusState private var focusedField: Field?
    private enum Field:Hashable {case server,username,password}
    var body: some View {
        @Bindable var store = store
        NavigationStack {
            Form {
                Section {
                    VStack(alignment:.leading,spacing:10) {
                        Image("Brand").resizable().frame(width:64,height:64).clipShape(RoundedRectangle(cornerRadius:14))
                        Text("APTEX").font(.system(size:36,weight:.light,design:.monospaced)).tracking(6)
                        Text("AUTONOMOUS PURPLE TEAM EXECUTION").font(.system(.caption2,design:.monospaced)).tracking(1.5).foregroundStyle(.secondary)
                        Text("by Afterglow Labs").font(.caption).foregroundStyle(.secondary)
                        Text("Connect to your desktop").font(.title2.weight(.semibold)).padding(.top,20)
                        Text("Your desktop runs assessments and models. This client connects to it.").foregroundStyle(.secondary).padding(.top,8)
                    }.padding(.vertical,20)
                }.listRowBackground(Color.clear)
                Section("Server address") {
                    TextField("https://desktop.tailnet.ts.net:8787",text:$store.serverText)
                        .textContentType(.URL).keyboardType(.URL).textInputAutocapitalization(.never).autocorrectionDisabled()
                        .focused($focusedField,equals:.server).accessibilityIdentifier("server-address")
                    TextField("Username",text:$store.username).textContentType(.username)
                        .focused($focusedField,equals:.username).textInputAutocapitalization(.never).autocorrectionDisabled().accessibilityIdentifier("username")
                    SecureField("Password",text:$password).textContentType(.password)
                        .textInputAutocapitalization(.never).autocorrectionDisabled()
                        .focused($focusedField,equals:.password).accessibilityIdentifier("password")
                }.disabled(store.busy)
                if let error=store.error {Section {ErrorBanner(message:error)}.listRowBackground(Color.clear)}
                Section {
                    Button {
                        focusedField=nil
                        Task {await store.login(password:password);if store.api != nil {password=""}}
                    } label: {
                        HStack {
                            if store.busy {ProgressView().tint(colorScheme == .dark ? .black : .white)}
                            Text("Connect").frame(maxWidth:.infinity)
                        }.foregroundStyle(colorScheme == .dark ? .black : .white).padding(.vertical,6)
                    }.buttonStyle(.borderedProminent).disabled(store.busy || password.isEmpty || store.serverText.isEmpty).accessibilityIdentifier("connect")
                    if !store.serverText.isEmpty {
                        Button("Reconnect with saved login") {Task {await store.restore()}}.disabled(store.busy)
                    }
                    Text("Use the same private DNS connection as your desktop setup, with APTEX's port.").font(.footnote).foregroundStyle(.secondary)
                }.listRowBackground(Color.clear)
            }.scrollDismissesKeyboard(.never).scrollContentBackground(.hidden)
                .frame(maxWidth:600).frame(maxWidth:.infinity).navigationTitle("")
        }
    }
}
struct WorkspaceView: View {
    @Environment(AppStore.self) private var store
    var body: some View {
        @Bindable var store = store
        TabView(selection: $store.selectedTab) {
            TasksView().tabItem { Label("Tasks", systemImage: "scope") }.tag(0)
            ConversationsView().tabItem { Label("Chat", systemImage: "bubble.left.and.text.bubble.right") }.tag(1)
            FindingsView().tabItem { Label("Findings", systemImage: "exclamationmark.shield") }.tag(2)
            ConsoleView().tabItem { Label("Console", systemImage: "terminal") }.tag(3)
            SettingsView().tabItem { Label("Settings", systemImage: "slider.horizontal.3") }.tag(4)
        }
    }
}
struct SettingsView: View {
    @Environment(AppStore.self) private var store
    @Environment(Appearance.self) private var appearance
    var body: some View {
        @Bindable var appearance = appearance
        NavigationStack {
            Form {
                Section("Connected server") {
                    LabeledContent("Account", value: store.username)
                    Text(store.serverText).font(.system(.footnote, design: .monospaced)).textSelection(.enabled)
                    LabeledContent("Server version", value: store.metadata?.server_version ?? "—")
                    LabeledContent("Client version", value: (Bundle.main.object(forInfoDictionaryKey:"CFBundleShortVersionString") as? String ?? "—") + " (" + (Bundle.main.object(forInfoDictionaryKey:"CFBundleVersion") as? String ?? "—") + ")")
                }
                Section("Appearance") {
                    Picker("Theme", selection: $appearance.preset) {
                        ForEach(["Obsidian", "Phosphor", "Afterglow"], id: \.self) { Text($0).tag($0) }
                    }.accessibilityIdentifier("theme-picker")
                    Picker("Color scheme", selection: $appearance.mode) {
                        Text("Dark").tag("dark"); Text("Light").tag("light"); Text("System").tag("system")
                    }
                    Picker("Language", selection: $appearance.language) {
                        Text("English").tag("en"); Text("한국어").tag("ko"); Text("简体中文").tag("zh-Hans")
                        Text("繁體中文").tag("zh-Hant"); Text("日本語").tag("ja"); Text("Español").tag("es")
                    }.accessibilityIdentifier("language-picker")
                }
                Section("Models and tools") {
                    Button("Configure models in Console") { store.openConsole("/system/llm") }
                    Text("Model profiles and access programs are managed by your server.").font(.footnote).foregroundStyle(.secondary)
                }
                Section {
                    Button("Switch server") { store.switchServer() }.disabled(store.busy)
                    Button("Sign out", role: .destructive) { Task { await store.logout() } }
                        .disabled(store.busy).accessibilityIdentifier("sign-out")
                    if let error = store.error { ErrorBanner(message: error) }
                }
                Section("About") {
                    Text("APTEX by Afterglow Labs").font(.system(.body, design: .monospaced))
                    Text("Client only. No server or model runtime runs on this device.").font(.footnote).foregroundStyle(.secondary)
                    NavigationLink("Licenses") { LicenseView() }
                }
            }.clientNavigationTitle("Settings").navigationBarTitleDisplayMode(.inline)
        }
    }
}
struct LicenseView: View {
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                Text("APTEX is licensed under AGPL-3.0. Selected client infrastructure is adapted from Afterglow Labs' Odysseus iOS client; see the source notices.")
                if let url = Bundle.main.url(forResource: "LICENSE", withExtension: nil), let text = try? String(contentsOf: url, encoding: .utf8) {
                    Text(text).font(.system(.caption, design: .monospaced)).textSelection(.enabled)
                }
            }.padding()
        }.clientNavigationTitle("Licenses")
    }
}
