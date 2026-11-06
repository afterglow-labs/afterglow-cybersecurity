import AppKit
import Foundation
let output = URL(fileURLWithPath: CommandLine.arguments[1], isDirectory: true)
for (points, factor) in [(16,1),(16,2),(32,1),(32,2),(128,1),(128,2),(256,1),(256,2),(512,1),(512,2)] {
    let pixels = points * factor
    guard let bitmap = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: pixels, pixelsHigh: pixels, bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0), let context = NSGraphicsContext(bitmapImageRep: bitmap) else { fatalError("Icon bitmap") }
    NSGraphicsContext.saveGraphicsState(); NSGraphicsContext.current = context
    let transform = NSAffineTransform(); transform.translateX(by: 0, yBy: CGFloat(pixels)); transform.scaleX(by: CGFloat(pixels)/64, yBy: -CGFloat(pixels)/64); transform.concat()
    NSColor(srgbRed: 0.047, green: 0.078, blue: 0.106, alpha: 1).setFill()
    NSBezierPath(roundedRect: NSRect(x: 3, y: 3, width: 58, height: 58), xRadius: 13, yRadius: 13).fill()
    NSColor(srgbRed: 0.404, green: 0.910, blue: 0.976, alpha: 1).setStroke()
    let arc = NSBezierPath(); arc.lineWidth = 4; arc.lineCapStyle = .round
    arc.move(to: NSPoint(x:18,y:37)); arc.curve(to:NSPoint(x:32,y:23), controlPoint1:NSPoint(x:18,y:29.268), controlPoint2:NSPoint(x:24.268,y:23)); arc.curve(to:NSPoint(x:46,y:37),controlPoint1:NSPoint(x:39.732,y:23),controlPoint2:NSPoint(x:46,y:29.268)); arc.stroke()
    for (x1,x2,y,width) in [(13.0,51.0,39.0,3.0),(19.0,45.0,46.0,2.5),(25.0,39.0,52.0,2.5)] {
        let line = NSBezierPath(); line.lineWidth = width; line.lineCapStyle = .round; line.move(to:NSPoint(x:x1,y:y)); line.line(to:NSPoint(x:x2,y:y)); line.stroke()
    }
    NSGraphicsContext.restoreGraphicsState()
    let filename = "icon_\(points)x\(points)" + (factor == 2 ? "@2x" : "") + ".png"
    try bitmap.representation(using: .png, properties: [:])!.write(to: output.appendingPathComponent(filename))
}
