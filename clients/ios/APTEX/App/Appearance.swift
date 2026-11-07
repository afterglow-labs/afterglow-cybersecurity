import SwiftUI
import Observation

@MainActor @Observable final class Appearance {
    var preset: String { didSet { UserDefaults.standard.set(preset, forKey: "aptex.theme") } }
    var mode: String { didSet { UserDefaults.standard.set(mode, forKey: "aptex.appearance") } }
    var language: String { didSet { UserDefaults.standard.set(language, forKey: "aptex.language") } }
    init() {
        preset = UserDefaults.standard.string(forKey: "aptex.theme") ?? "Obsidian"
        mode = UserDefaults.standard.string(forKey: "aptex.appearance") ?? "dark"
        language = UserDefaults.standard.string(forKey: "aptex.language") ?? "en"
    }
    var accent:Color {
        switch preset {
        case "Phosphor":return Self.phosphor
        case "Afterglow":return Self.afterglow
        default:return Self.obsidian
        }
    }
    private static let phosphor=dynamicAccent(light:UIColor(red:0.086,green:0.471,blue:0.267,alpha:1),dark:UIColor(red:0.361,green:0.922,blue:0.635,alpha:1))
    private static let afterglow=dynamicAccent(light:UIColor(red:0.643,green:0.322,blue:0.086,alpha:1),dark:UIColor(red:0.961,green:0.718,blue:0.361,alpha:1))
    private static let obsidian=dynamicAccent(light:UIColor(red:0.055,green:0.455,blue:0.565,alpha:1),dark:UIColor(red:0.404,green:0.910,blue:0.976,alpha:1))
    private static func dynamicAccent(light:UIColor,dark:UIColor)->Color {Color(uiColor:UIColor {$0.userInterfaceStyle == .dark ? dark : light})}
    var scheme: ColorScheme? { mode == "system" ? nil : mode == "light" ? .light : .dark }
}

struct StatusBadge: View {
    let value: String
    var body: some View {
        Text(LocalizedStringKey(value.replacingOccurrences(of: "_", with: " ").capitalized))
            .font(.system(.caption2, design: .monospaced).weight(.semibold))
            .foregroundStyle(color).padding(.horizontal, 7).padding(.vertical, 4)
            .background(color.opacity(0.12), in: Capsule())
    }
    var color: Color {
        switch value { case "critical", "failed", "error": return .red
        case "high", "paused", "pending": return .orange
        case "running", "in_progress": return .cyan
        case "done", "completed", "resolved", "confirmed": return .green
        default: return .secondary }
    }
}
struct ErrorBanner: View {
    let message: String
    var body: some View {
        Label { Text(LocalizedStringKey(message)).font(.callout).textSelection(.enabled) } icon: { Image(systemName: "exclamationmark.triangle") }
            .foregroundStyle(.orange).frame(maxWidth: .infinity, alignment: .leading)
            .padding(12).background(.orange.opacity(0.08), in: RoundedRectangle(cornerRadius: 10))
            .accessibilityIdentifier("error-banner")
    }
}
struct ContentText: View {
    let value: String
    var body: some View {
        Text(String(value.prefix(100_000))).font(.system(.body, design: .monospaced)).textSelection(.enabled)
            .frame(maxWidth: .infinity, alignment: .leading)
        if value.count > 100_000 { Text("Open complete content in Console.").font(.caption).foregroundStyle(.secondary) }
    }
}

private struct ClientNavigationTitle: ViewModifier {
    let key: String
    @Environment(\.locale) private var locale
    func body(content: Content) -> some View {
        content.navigationTitle(Text(verbatim: clientLocalizedText(key, language: locale.identifier)))
    }
}
extension View {
    func clientNavigationTitle(_ key: String) -> some View { modifier(ClientNavigationTitle(key: key)) }
}

func clientLocalizedText(_ key: String, language: String) -> String {
    let requested=language.replacingOccurrences(of:"_",with:"-")
    let supported=["zh-Hans","zh-Hant","ko","ja","es","en"]
    let selected=supported.first { requested == $0 || requested.hasPrefix($0+"-") } ?? "en"
    guard let path=Bundle.main.path(forResource:selected,ofType:"lproj"),let bundle=Bundle(path:path) else {return key}
    return bundle.localizedString(forKey:key,value:key,table:"Localizable")
}
