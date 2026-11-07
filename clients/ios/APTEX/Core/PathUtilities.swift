// Adapted from afterglow-labs/odysseus-ios f81c2cd09b90, AGPL-3.0.
import Foundation

extension String {
    var pathComponent: String { addingPercentEncoding(withAllowedCharacters: .urlPathAllowed.subtracting(CharacterSet(charactersIn: "/?#%"))) ?? self }
}
