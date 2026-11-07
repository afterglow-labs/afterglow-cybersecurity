// Render the existing APTEX vector mark as an opaque iOS app icon.
import AppKit
import Foundation
let output = URL(fileURLWithPath: CommandLine.arguments[1], isDirectory: true)
for (points, factor) in [(1024,1)] {
    let pixels = points * factor
    guard let canvas = CGContext(data: nil, width: pixels, height: pixels, bitsPerComponent: 8, bytesPerRow: 0, space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue) else { fatalError("Icon canvas") }
    let context = NSGraphicsContext(cgContext: canvas, flipped: false)
    NSGraphicsContext.saveGraphicsState(); NSGraphicsContext.current = context
    let transform = NSAffineTransform(); transform.translateX(by: 0, yBy: CGFloat(pixels)); transform.scaleX(by: CGFloat(pixels)/64, yBy: -CGFloat(pixels)/64); transform.concat()
    NSColor(srgbRed: 0.047, green: 0.078, blue: 0.106, alpha: 1).setFill()
    NSBezierPath(rect: NSRect(x: 0, y: 0, width: 64, height: 64)).fill()
    NSColor(srgbRed: 0.404, green: 0.910, blue: 0.976, alpha: 1).setStroke()
    let arc = NSBezierPath(); arc.lineWidth = 4; arc.lineCapStyle = .round
    arc.move(to: NSPoint(x:18,y:37)); arc.curve(to:NSPoint(x:32,y:23), controlPoint1:NSPoint(x:18,y:29.268), controlPoint2:NSPoint(x:24.268,y:23)); arc.curve(to:NSPoint(x:46,y:37),controlPoint1:NSPoint(x:39.732,y:23),controlPoint2:NSPoint(x:46,y:29.268)); arc.stroke()
    for (x1,x2,y,width) in [(13.0,51.0,39.0,3.0),(19.0,45.0,46.0,2.5),(25.0,39.0,52.0,2.5)] {
        let line = NSBezierPath(); line.lineWidth = width; line.lineCapStyle = .round; line.move(to:NSPoint(x:x1,y:y)); line.line(to:NSPoint(x:x2,y:y)); line.stroke()
    }
    NSGraphicsContext.restoreGraphicsState()
    let filename = "AppIcon.png"
    guard let image = canvas.makeImage(), let data = NSBitmapImageRep(cgImage: image).representation(using: .png, properties: [:]) else { fatalError("Icon encoding") }
    try data.write(to: output.appendingPathComponent(filename))
}
