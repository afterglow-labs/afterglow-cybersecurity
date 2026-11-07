import SwiftUI
import UniformTypeIdentifiers
import PhotosUI

struct ConversationsView: View {
    @Environment(AppStore.self) private var store
    @State private var conversations: [Conversation] = []
    @State private var creating = false
    @State private var loading = true
    @State private var error: String?
    @State private var path: [Conversation] = []
    @State private var search = ""
    @State private var selection: Conversation?
    @Environment(\.horizontalSizeClass) private var sizeClass
    var body: some View {
        Group {
            if sizeClass == .regular {
                NavigationSplitView {
                    conversationList.navigationSplitViewColumnWidth(min:240,ideal:290,max:360)
                } detail: {
                    if let selection { ChatDetailHost(conversation:selection).id(selection.id) }
                    else {ContentUnavailableView("Start a conversation",systemImage:"bubble.left.and.text.bubble.right")}
                }
            } else {
                NavigationStack(path:$path) {
                    conversationList.navigationDestination(for:Conversation.self) { ChatDetailHost(conversation:$0) }
                }
            }
        }.task {await load()}
            .sheet(isPresented:$creating) {
                CreateConversationView { conversation in
                    conversations.insert(conversation,at:0)
                    if sizeClass == .regular {selection=conversation} else {path.append(conversation)}
                }
            }
    }
    private var conversationList: some View {
        List {
            if let error {ErrorBanner(message:error)}
            if loading && conversations.isEmpty {ProgressView("Loading conversations…")}
            if !loading && conversations.isEmpty && error == nil {
                ContentUnavailableView("Start a conversation",systemImage:"bubble.left.and.text.bubble.right",description:Text("Choose an agent and model profile to work with your APTEX server."))
            }
            ForEach(conversations.filter {search.isEmpty || $0.title.localizedCaseInsensitiveContains(search) || $0.agent_key.localizedCaseInsensitiveContains(search)}) { conversation in
                Button {
                    if sizeClass == .regular {selection=conversation} else {path.append(conversation)}
                } label: {
                    VStack(alignment:.leading,spacing:7) {
                        HStack {Text(conversation.title).font(.headline);Spacer();if conversation.running == true {StatusBadge(value:"running")}}
                        Text(conversation.agent_key).font(.system(.caption,design:.monospaced)).foregroundStyle(.secondary)
                    }.padding(.vertical,6)
                }.buttonStyle(.plain).accessibilityIdentifier("conversation-"+String(conversation.id))
            }
        }.clientNavigationTitle("Chat").navigationBarTitleDisplayMode(.inline)
            .searchable(text:$search,prompt:"Search conversations")
            .toolbar {Button {creating=true} label:{Label("New conversation",systemImage:"square.and.pencil")}.accessibilityIdentifier("new-conversation")}
            .refreshable {await load()}
    }
    private func load() async {
        guard let api = store.api else { return }
        loading = true; defer { loading = false }
        do { let list: ConversationList = try await api.get("/api/v1/conversations"); conversations = list.conversations; error = nil }
        catch { self.error = error.localizedDescription; store.handle(error, from: api) }
    }
}
struct CreateConversationView: View {
    @Environment(AppStore.self) private var store
    @Environment(\.dismiss) private var dismiss
    let created: (Conversation) -> Void
    @State private var agents: [AgentSummary] = []
    @State private var profiles: [ModelProfile] = []
    @State private var agent = ""
    @State private var profile = ""
    @State private var title = ""
    @State private var error: String?
    @State private var busy = false
    var body: some View {
        NavigationStack {
            Form {
                TextField("Title (optional)", text: $title)
                Picker("Agent", selection: $agent) { ForEach(agents) { Text($0.name).tag($0.key) } }
                Section("Model") { ProfilePicker(profiles: profiles, selection: $profile) }
                if let error { ErrorBanner(message: error) }
            }.clientNavigationTitle("New conversation").navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() }.disabled(busy) }
                    ToolbarItem(placement: .confirmationAction) { Button("Create") { Task { await create() } }.disabled(busy || agent.isEmpty).accessibilityIdentifier("create-conversation") }
                }.interactiveDismissDisabled(busy).task { await load() }
        }
    }
    private func load() async {
        guard let api = store.api else { return }
        do {
            let list: AgentList = try await api.get("/api/v1/agents")
            agents = list.agents.filter { $0.enabled != false }
            agent = agents.first(where: { $0.key == "mainagent" })?.key ?? agents.first?.key ?? ""
            let models: ProfileList = try await api.get("/api/v1/llm/profiles"); profiles = models.profiles
        } catch { self.error = error.localizedDescription; store.handle(error, from: api) }
    }
    private func create() async {
        guard let api = store.api, !busy else { return }
        busy = true; defer { busy = false }
        do {
            let conversation: Conversation = try await api.send("/api/v1/conversations", body: NewConversation(agent_key: agent, title: title, llm_profile_id: try selectedProfileID(profile)))
            dismiss(); created(conversation)
        } catch { self.error = error.localizedDescription; store.handle(error, from: api) }
    }
}
struct ChatDetailHost: View {
    @Environment(AppStore.self) private var store
    let conversation: Conversation
    @State private var model: ChatSession?
    var body: some View {
        Group { if let model { ChatDetailView(model: model) } else { ProgressView() } }
            .task { if model == nil { model = store.chat(conversation) } }
    }
}
struct ChatDetailView: View {
    @Environment(AppStore.self) private var store
    @Environment(\.scenePhase) private var phase
    @Bindable var model: ChatSession
    @State private var importing = false
    @State private var choosingPhotos = false
    @State private var photos: [PhotosPickerItem] = []
    @State private var importingPhotos = false
    @State private var selectingModel = false
    @State private var profiles: [ModelProfile] = []
    @State private var selectedProfile = ""
    @FocusState private var composerFocused: Bool
    var body: some View {
        ScrollViewReader { scroll in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 18) {
                    if model.hasMore { Button("Load earlier messages") { Task { await model.loadEarlier() } }.disabled(model.loading) }
                    if model.loading { ProgressView("Loading history…") }
                    ForEach(model.activities) { item in
                        VStack(alignment: .leading, spacing: 8) {
                            HStack {
                                Text(LocalizedStringKey(item.isUser ? "You" : "Agent")).font(.system(.caption2, design: .monospaced).weight(.bold)).foregroundStyle(.secondary)
                                if let tool = item.tool, !tool.isEmpty { Text(tool).font(.system(.caption2, design: .monospaced)).foregroundStyle(.tint) }
                                Spacer()
                                if item.is_error == true { Image(systemName: "exclamationmark.circle").foregroundStyle(.red) }
                            }
                            if item.isMessage { MessageBody(text:item.body).equatable() }
                            else {DisclosureGroup(item.kind ?? "Activity") {ContentText(value:item.body)}}
                            Button {UIPasteboard.general.string=item.body} label:{Label("Copy message",systemImage:"doc.on.doc")}.font(.caption)
                            if item.detail == nil || item.detail?.isEmpty == true {
                                Button("Details") { Task { await model.expand(item) } }.font(.caption)
                            }
                        }.task(id:String(item.id)+":"+(model.controls?.history_revision ?? "")) {if item.isMessage {await model.expand(item)}}
                            .padding(14).frame(maxWidth: .infinity, alignment: .leading)
                            .background(.quaternary.opacity(item.isUser ? 0.8 : 0.3), in: RoundedRectangle(cornerRadius: 12))
                            .id(item.id)
                    }
                    if model.running { Label("Running on server", systemImage: "waveform.path").font(.caption).foregroundStyle(.tint) }
                    if let controls = model.controls {
                        ForEach(controls.inputs.filter { ["queued", "pending", "admitted", "unknown"].contains($0.status) }) { input in
                            VStack(alignment: .leading) { StatusBadge(value: input.status); Text(input.prompt.text).font(.callout).lineLimit(3) }
                        }
                    }
                    if let error = model.error { ErrorBanner(message: error) }
                    if model.pending != nil {
                        Text("Send outcome is unconfirmed. Retry safely with the same request.").font(.footnote).foregroundStyle(.orange)
                    }
                    Color.clear.frame(height: 1).id("latest")
                }.padding()
            }.defaultScrollAnchor(.bottom)
                .onChange(of: model.activities.last?.id) { _, _ in
                    // New messages follow while the composer is at the bottom. Older
                    // pagination leaves the scroll position anchored to existing rows.
                    if model.running || model.sending { withAnimation { scroll.scrollTo("latest", anchor: .bottom) } }
                }
        }
        .scrollDismissesKeyboard(.interactively)
        .safeAreaInset(edge: .bottom,spacing:0) { composer }
        .navigationTitle(model.conversation.title).navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button("Model profile") { selectedProfile=model.profileID.map(String.init) ?? "";selectingModel = true }
                    Button("Open conversation in Console") { store.openConsole("/chat?c=\(model.conversation.id)") }
                    Button("Refresh history") { Task { await model.refresh() } }
                } label: { Image(systemName: "ellipsis.circle") }
            }
        }
        .task(id: phase) { if phase == .active { await model.startObserving() } else { model.stopObserving() } }
        .onDisappear { model.stopObserving() }
        .sheet(isPresented:$importing) {
            AttachmentFilePicker {files in importing=false;model.startUpload(files)}
        }
        .photosPicker(isPresented:$choosingPhotos,selection:$photos,maxSelectionCount:8,matching:.images)
        .task(id:photos) {
            guard !photos.isEmpty else {return}
            importingPhotos=true;defer {importingPhotos=false;photos=[]}
            var importedFiles:[URL]=[]
            do {
                for photo in photos {
                    try Task.checkCancellation()
                    guard let imported=try await photo.loadTransferable(type:ImportedPhoto.self) else {continue}
                    importedFiles.append(imported.url)
                }
                try Task.checkCancellation()
                model.startUpload(importedFiles,removeImportedCopies:true)
            } catch {
                for file in importedFiles {try? FileManager.default.removeItem(at:file.deletingLastPathComponent())}
                if !(error is CancellationError) {model.error=error.localizedDescription}
            }
        }
        .sheet(isPresented: $selectingModel) {
            NavigationStack {
                Form {
                    ProfilePicker(profiles: profiles, selection: $selectedProfile)
                    if let error = model.error { ErrorBanner(message: error) }
                    Button("Configure profiles in Console") { selectingModel = false; store.openConsole("/system/llm") }
                }.clientNavigationTitle("Model profile").navigationBarTitleDisplayMode(.inline)
                    .toolbar {
                        ToolbarItem(placement: .cancellationAction) { Button("Cancel") { selectingModel = false } }
                        ToolbarItem(placement: .confirmationAction) { Button("Apply") { Task { if await model.selectProfile(selectedProfile) { selectingModel = false } } }.disabled(model.running || model.sending || model.pending != nil) }
                    }
            }
        }
        .task {
            selectedProfile = model.profileID.map(String.init) ?? ""
            if let api = store.api { do { let models: ProfileList = try await api.get("/api/v1/llm/profiles"); profiles = models.profiles } catch { model.error = error.localizedDescription; store.handle(error, from: api) } }
        }
    }
    private var composer: some View {
        VStack(spacing: 8) {
            HStack {
                Button {selectedProfile=model.profileID.map(String.init) ?? "";selectingModel=true} label:{
                    HStack(spacing:5) {Image(systemName:"cpu");Text(profiles.first(where:{$0.id==(model.profileID.map(String.init) ?? "")})?.label ?? "Server default").lineLimit(1);Image(systemName:"chevron.down").font(.caption2)}
                }.font(.caption).frame(minHeight:44).accessibilityIdentifier("model-picker").disabled(model.running || model.sending || model.pending != nil)
                Spacer()
                if composerFocused {
                    Button {composerFocused=false} label:{Image(systemName:"keyboard.chevron.compact.down").frame(width:44,height:44)}
                        .accessibilityLabel("Done").accessibilityIdentifier("dismiss-keyboard")
                }
                Button {store.openConsole("/system/llm")} label:{Image(systemName:"slider.horizontal.3").frame(width:44,height:44)}.accessibilityLabel("Model controls")
            }
            if importingPhotos {Label("Importing attachment…",systemImage:"photo").font(.caption).foregroundStyle(.secondary)}
            if model.uploading {
                HStack {ProgressView().controlSize(.small);Text(model.uploadName).font(.caption).lineLimit(1);Spacer();Button("Cancel upload"){model.cancelUpload()}.font(.caption)}
            }
            if !model.attachments.isEmpty {
                ScrollView(.horizontal) {
                    HStack {
                        ForEach(model.attachments) { attachment in
                            Button { model.attachments.removeAll { $0.id == attachment.id } } label: {
                                Label(attachment.name, systemImage: "xmark.circle").font(.caption)
                            }.disabled(model.pending != nil)
                        }
                    }
                }
            }
            HStack(alignment: .bottom, spacing: 12) {
                Menu {
                    Button("Choose files",systemImage:"doc"){composerFocused=false;importing=true}
                    Button("Choose photos",systemImage:"photo"){composerFocused=false;choosingPhotos=true}
                } label:{Image(systemName:"paperclip").frame(width:44,height:44)}
                    .accessibilityLabel("Attach files").disabled(model.uploading || importingPhotos || model.pending != nil || model.sending)
                TextField("Message", text: $model.draft, axis: .vertical).lineLimit(1...8).padding(10)
                    .focused($composerFocused)
                    .background(.quaternary, in: RoundedRectangle(cornerRadius: 12))
                    .disabled(model.pending != nil || model.sending).accessibilityIdentifier("message-draft")
                Button { composerFocused=false;Task { await model.send() } } label: {
                    if model.sending || model.uploading || importingPhotos { ProgressView().frame(width: 44, height: 44) }
                    else { Image(systemName: model.pending == nil ? "arrow.up.circle.fill" : "arrow.clockwise.circle.fill").font(.title).frame(width: 44, height: 44) }
                }.keyboardShortcut(.return,modifiers:.command).accessibilityLabel(model.pending == nil ? (model.running ? "Queue message" : "Send message") : "Retry send")
                    .accessibilityIdentifier("send-message")
                    .disabled(model.sending || model.uploading || importingPhotos || model.draft.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && model.attachments.isEmpty && model.pending == nil)
            }
            HStack {
                Label(LocalizedStringKey(model.streamConnected ? "Live" : "Connecting…"), systemImage: model.streamConnected ? "circle.fill" : "arrow.triangle.2.circlepath")
                    .font(.system(.caption2, design: .monospaced)).foregroundStyle(.secondary)
                Spacer()
                if let controls = model.controls, controls.running || controls.paused {
                    Button(controls.paused ? "Resume" : "Pause") { Task { await model.control() } }.font(.caption).disabled(model.sending)
                }
            }
        }.padding(.horizontal).padding(.vertical, 10).background(.bar)
    }
}
