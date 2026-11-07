// Adapted from Odysseus iOS f81c2cd09b90, AGPL-3.0.
import SwiftUI
import PhotosUI
import UniformTypeIdentifiers
import CoreTransferable

struct AttachmentFilePicker: UIViewControllerRepresentable {
    @Environment(\.dismiss) private var dismiss
    let picked: ([URL]) -> Void
    func makeUIViewController(context:Context)->UIDocumentPickerViewController {
        let picker=UIDocumentPickerViewController(forOpeningContentTypes:[.data],asCopy:true)
        picker.allowsMultipleSelection=true;picker.delegate=context.coordinator
        return picker
    }
    func updateUIViewController(_ controller:UIDocumentPickerViewController,context:Context) {}
    func makeCoordinator()->Coordinator {Coordinator(self)}
    final class Coordinator:NSObject,UIDocumentPickerDelegate {
        let parent:AttachmentFilePicker
        init(_ parent:AttachmentFilePicker){self.parent=parent}
        func documentPicker(_ controller:UIDocumentPickerViewController,didPickDocumentsAt urls:[URL]) {parent.picked(urls);parent.dismiss()}
        func documentPickerWasCancelled(_ controller:UIDocumentPickerViewController) {parent.dismiss()}
    }
}
struct ImportedPhoto:Transferable {
    let url:URL
    static var transferRepresentation:some TransferRepresentation {
        FileRepresentation(importedContentType:.image) {received in
            let size=try received.file.resourceValues(forKeys:[.fileSizeKey]).fileSize ?? 0
            guard size<=32*1024*1024 else {throw ClientError.message("Choose a file smaller than 32 MB, or upload it in Console.")}
            let directory=FileManager.default.temporaryDirectory.appendingPathComponent("aptex-photo-"+UUID().uuidString,isDirectory:true)
            try FileManager.default.createDirectory(at:directory,withIntermediateDirectories:true,attributes:[.protectionKey:FileProtectionType.complete])
            let target=directory.appendingPathComponent(received.file.lastPathComponent)
            do {try FileManager.default.copyItem(at:received.file,to:target);try FileManager.default.setAttributes([.protectionKey:FileProtectionType.complete],ofItemAtPath:target.path)}
            catch {try? FileManager.default.removeItem(at:directory);throw error}
            return ImportedPhoto(url:target)
        }
    }
}
