import SwiftUI

@main struct APTEXApp: App {
    @State private var store = AppStore()
    @State private var appearance = Appearance()
    var body: some Scene {
        WindowGroup {
            RootView().environment(store).environment(appearance)
                .tint(appearance.accent).preferredColorScheme(appearance.scheme)
                .environment(\.locale, Locale(identifier: appearance.language))
                .task { await store.restore() }
                .environment(\.openURL,OpenURLAction {url in
                    guard let api=store.api else {return .discarded}
                    let target=url.scheme==nil ? URL(string:url.relativeString,relativeTo:api.address.origin)?.absoluteURL : url
                    guard let target,target.user==nil,target.password==nil else {return .discarded}
                    if api.address.contains(target) {store.openConsole(target.path+(target.query.map{"?"+$0} ?? ""));return .handled}
                    guard ["http","https"].contains(target.scheme ?? "") else {return .discarded}
                    UIApplication.shared.open(target);return .handled
                })
                .onOpenURL { _ in } // No URL-based credential or command import.
        }
    }
}
