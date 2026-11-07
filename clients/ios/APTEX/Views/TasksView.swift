import SwiftUI

struct TasksView: View {
    @Environment(AppStore.self) private var store
    @State private var tasks: [SecurityTask] = []
    @State private var search = ""
    @State private var loading = true
    @State private var error: String?
    @State private var creating = false
    var body: some View {
        NavigationStack {
            List {
                if let error { ErrorBanner(message: error) }
                if loading && tasks.isEmpty { ProgressView("Loading tasks…") }
                if !loading && tasks.isEmpty && error == nil {
                    ContentUnavailableView("No tasks yet", systemImage: "scope", description: Text("Create a task or use the full Console to configure an assessment."))
                }
                ForEach(tasks.filter { search.isEmpty || $0.title.localizedCaseInsensitiveContains(search) || $0.description.localizedCaseInsensitiveContains(search) }) { task in
                    NavigationLink(value: task) {
                        VStack(alignment: .leading, spacing: 8) {
                            HStack(alignment: .top) { Text(task.title).font(.headline).lineLimit(2); Spacer(); StatusBadge(value: task.status) }
                            Text(task.goal).font(.subheadline).foregroundStyle(.secondary).lineLimit(2)
                            if let met = task.goals_met, let total = task.goals_total {
                                HStack { Label("\(met)/\(total)", systemImage: "checkmark.circle"); Spacer(); Text("#" + task.id) }
                                    .font(.system(.caption, design: .monospaced)).foregroundStyle(.secondary)
                            }
                        }.padding(.vertical, 6)
                    }
                }
            }.listStyle(.plain).clientNavigationTitle("Tasks").navigationBarTitleDisplayMode(.inline).searchable(text: $search, prompt: "Search tasks")
                .navigationDestination(for: SecurityTask.self) { TaskDetailView(task: $0) }
                .toolbar { Button { creating = true } label: { Label("New task", systemImage: "plus") }.accessibilityIdentifier("new-task") }
                .refreshable { await load() }
                .task { await load() }
                .sheet(isPresented: $creating, onDismiss: { Task { await load() } }) { CreateTaskView() }
        }
    }
    private func load() async {
        guard let api = store.api else { return }
        loading = true; defer { loading = false }
        do { let result: TaskList = try await api.get("/api/v1/tasks"); tasks = result.tasks; error = nil }
        catch { self.error = error.localizedDescription; store.handle(error, from: api) }
    }
}
struct TaskDetailView: View {
    @Environment(AppStore.self) private var store
    let task: SecurityTask
    @State private var current: SecurityTask?
    @State private var goals: [TaskGoal] = []
    @State private var constraints: [TaskConstraint] = []
    @State private var error: String?
    @State private var busy = false
    @Environment(\.scenePhase) private var phase
    var shown: SecurityTask { current ?? task }
    var body: some View {
        List {
            if let error { ErrorBanner(message: error) }
            Section {
                StatusBadge(value: shown.status)
                ContentText(value: shown.description)
            }
            Section("Goal") { ContentText(value: shown.goal) }
            if !goals.isEmpty {
                Section("Progress") {
                    ForEach(goals) { goal in
                        VStack(alignment: .leading, spacing: 6) { Text(goal.text); StatusBadge(value: goal.state) }
                    }
                }
            }
            if !constraints.isEmpty {
                Section("Scope and constraints") {
                    ForEach(constraints) { rule in VStack(alignment: .leading) { StatusBadge(value: rule.kind); Text(rule.text) } }
                }
            }
            Section {
                if shown.status == "running" || shown.status == "paused" {
                    Button(shown.status == "paused" ? "Resume task" : "Pause task") { Task { await control() } }.disabled(busy)
                }
                Button("Open full task in Console") { store.openConsole("/function/tasks/detail?id=" + task.id.pathComponent) }
            }
        }.navigationTitle(shown.title).navigationBarTitleDisplayMode(.inline)
            .refreshable { await load() }
            .task(id: phase) {
                guard phase == .active else { return }
                while !Task.isCancelled { await load(); do { try await Task.sleep(for: .seconds(5)) } catch { return } }
            }
    }
    private func load() async {
        guard let api = store.api else { return }
        do {
            current = try await api.get("/api/v1/tasks/" + task.id.pathComponent)
            let goalList: GoalList = try await api.get("/api/v1/tasks/\(task.id.pathComponent)/goals")
            let ruleList: ConstraintList = try await api.get("/api/v1/tasks/\(task.id.pathComponent)/constraints")
            goals = goalList.goals; constraints = ruleList.constraints; error = nil
        } catch is CancellationError {} catch { self.error = error.localizedDescription; store.handle(error, from: api) }
    }
    private func control() async {
        guard let api = store.api, !busy else { return }
        busy = true; defer { busy = false }
        do {
            let _: EmptyResponse = try await api.send("/api/v1/tasks/\(task.id.pathComponent)/control", body: TaskAction(action: shown.status == "paused" ? "resume" : "pause"))
            await load()
        } catch { self.error = error.localizedDescription; store.handle(error, from: api) }
    }
}
struct CreateTaskView: View {
    @Environment(AppStore.self) private var store
    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @State private var description = ""
    @State private var goal = ""
    @State private var profile = ""
    @State private var profiles: [ModelProfile] = []
    @State private var busy = false
    @State private var uncertain = false
    @State private var error: String?
    var body: some View {
        NavigationStack {
            Form {
                Section("Task") {
                    TextField("Name (optional)", text: $name)
                    TextField("Assessment instructions", text: $description, axis: .vertical).lineLimit(4...10)
                    TextField("Goal", text: $goal, axis: .vertical).lineLimit(2...5)
                }
                Section("Model") { ProfilePicker(profiles: profiles, selection: $profile) }
                Section {
                    Text("Creating this task starts execution on your server.").font(.footnote)
                    Button("Advanced scope and interception in Console") { dismiss(); store.openConsole("/function/tasks") }
                }
                if let error { ErrorBanner(message: error) }
                if uncertain { Text("Check Tasks or Console before creating another task. The creation result is unconfirmed.").font(.footnote).foregroundStyle(.orange) }
            }.clientNavigationTitle("New task").navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() }.disabled(busy) }
                    ToolbarItem(placement: .confirmationAction) {
                        Button("Create and run") { Task { await create() } }.disabled(busy || uncertain || description.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || goal.isEmpty)
                    }
                }.interactiveDismissDisabled(busy)
                .task { if let api = store.api { do { let list: ProfileList = try await api.get("/api/v1/llm/profiles"); profiles = list.profiles } catch { self.error = error.localizedDescription; store.handle(error, from: api) } } }
        }
    }
    private func create() async {
        guard let api = store.api, !busy, !uncertain else { return }
        let profileID: Int64?
        do {profileID=try selectedProfileID(profile)} catch {self.error=error.localizedDescription;return}
        busy = true; defer { busy = false }
        do {
            let _: EmptyResponse = try await api.send("/api/v1/tasks", body: NewTask(name: name, description: description, goal: goal, llm_profile_id: profileID, timeout_seconds: 3600))
            dismiss()
        } catch {
            uncertain=(error as? ClientError)?.ambiguous ?? true
            self.error = error.localizedDescription; store.handle(error, from: api)
        }
    }
}
struct ProfilePicker: View {
    let profiles: [ModelProfile]
    @Binding var selection: String
    var body: some View {
        Picker("Model profile", selection: $selection) {
            Text("Server default").tag("")
            ForEach(profiles) { profile in
                Text(profile.label + ((profile.access_program_cyber?.isEmpty == false) ? " · " + profile.access_program_cyber!.capitalized : ""))
                    .tag(profile.id).disabled(!profile.available)
            }
        }.accessibilityIdentifier("profile-selection")
        if profiles.isEmpty { Text("Configure model profiles in Console to choose a model here.").font(.caption).foregroundStyle(.secondary) }
    }
}
