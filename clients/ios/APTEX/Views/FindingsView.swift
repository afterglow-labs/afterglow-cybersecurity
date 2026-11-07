import SwiftUI

struct FindingsView: View {
    @Environment(AppStore.self) private var store
    @State private var findings: [Finding] = []
    @State private var page = 1
    @State private var total = 0
    @State private var loading = true
    @State private var error: String?
    @State private var search = ""
    var body: some View {
        NavigationStack {
            List {
                if let error { ErrorBanner(message: error) }
                if loading && findings.isEmpty { ProgressView("Loading findings…") }
                if !loading && findings.isEmpty && error == nil { ContentUnavailableView("No findings yet", systemImage: "checkmark.shield", description: Text("Findings and evidence from your assessments appear here.")) }
                ForEach(findings.filter { search.isEmpty || $0.title.localizedCaseInsensitiveContains(search) || $0.summary.localizedCaseInsensitiveContains(search) }) { finding in
                    NavigationLink(value: finding) {
                        VStack(alignment: .leading, spacing: 8) {
                            HStack { Text(finding.title).font(.headline); Spacer(); StatusBadge(value: finding.severity) }
                            Text(finding.summary).font(.subheadline).foregroundStyle(.secondary).lineLimit(3)
                            StatusBadge(value: finding.status)
                        }.padding(.vertical, 6)
                    }
                }
                if findings.count < total { Button("Load more") { Task { await load(reset: false) } }.disabled(loading) }
            }.clientNavigationTitle("Findings").navigationBarTitleDisplayMode(.inline).searchable(text: $search, prompt: "Search loaded findings")
                .navigationDestination(for: Finding.self) { FindingDetailView(finding: $0) }
                .task { await load(reset: true) }.refreshable { await load(reset: true) }
        }
    }
    private func load(reset: Bool) async {
        guard let api = store.api else { return }
        loading = true; defer { loading = false }
        let next = reset ? 1 : page + 1
        do {
            let result: FindingPage = try await api.get("/api/v1/exploration/findings?page=\(next)&limit=100")
            findings = reset ? result.items : findings + result.items.filter { item in !findings.contains { $0.id == item.id } }
            total = result.total; page = next; error = nil
        } catch { self.error = error.localizedDescription; store.handle(error, from: api) }
    }
}
struct FindingDetailView: View {
    @Environment(AppStore.self) private var store
    let finding: Finding
    @State private var detail: Finding?
    @State private var error: String?
    var shown: Finding { detail ?? finding }
    var body: some View {
        List {
            if let error { ErrorBanner(message: error) }
            Section { HStack { StatusBadge(value: shown.severity); StatusBadge(value: shown.status) }; ContentText(value: shown.summary) }
            if !shown.evidence.isEmpty { Section("Evidence") { ContentText(value: shown.evidence) } }
            if let report = shown.report, !report.isEmpty { Section("Report") { ContentText(value: report) } }
            Section { Button("Open complete finding in Console") { store.openConsole("/function/findings/detail?id=" + shown.id.pathComponent) } }
        }.navigationTitle(shown.title).navigationBarTitleDisplayMode(.inline).task {
            guard let api = store.api else { return }
            do { detail = try await api.get("/api/v1/exploration/findings/" + finding.id.pathComponent) }
            catch { self.error = error.localizedDescription; store.handle(error, from: api) }
        }
    }
}
