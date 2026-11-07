// Markdown/code presentation follows Odysseus iOS f81c2cd09b90, AGPL-3.0.
import SwiftUI

struct MessageBody: View, Equatable {
    let text: String
    var body: some View {
        let segments=String(text.prefix(100_000)).components(separatedBy:"```")
        VStack(alignment:.leading,spacing:10) {
            ForEach(segments.indices,id:\.self) { index in
                if index % 2 == 1 {
                    ScrollView(.horizontal) {
                        Text(code(segments[index])).font(.system(.callout,design:.monospaced)).textSelection(.enabled)
                            .padding(12).frame(maxWidth:.infinity,alignment:.leading)
                    }.background(.quaternary,in:RoundedRectangle(cornerRadius:8))
                } else if !segments[index].isEmpty {
                    Text((try? AttributedString(markdown:segments[index],options:.init(interpretedSyntax:.inlineOnlyPreservingWhitespace))) ?? AttributedString(segments[index]))
                        .textSelection(.enabled).frame(maxWidth:.infinity,alignment:.leading)
                }
            }
            if text.count>100_000 {Text("Open complete content in Console.").font(.caption).foregroundStyle(.secondary)}
        }
    }
    private func code(_ value:String)->String {
        guard let newline=value.firstIndex(of:"\n") else {return value}
        let language=value[..<newline]
        if language.count<30 && !language.contains(" ") {return String(value[value.index(after:newline)...])}
        return value
    }
}
