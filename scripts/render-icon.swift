import AppKit

guard CommandLine.arguments.count == 3 else {
    fputs("Usage: swift scripts/render-icon.swift source.svg output.png\n", stderr)
    exit(1)
}

let source = CommandLine.arguments[1]
let destination = CommandLine.arguments[2]
guard let image = NSImage(contentsOfFile: source),
      let bitmap = NSBitmapImageRep(
        bitmapDataPlanes: nil,
        pixelsWide: 1024,
        pixelsHigh: 1024,
        bitsPerSample: 8,
        samplesPerPixel: 4,
        hasAlpha: true,
        isPlanar: false,
        colorSpaceName: .deviceRGB,
        bytesPerRow: 0,
        bitsPerPixel: 0
      ),
      let context = NSGraphicsContext(bitmapImageRep: bitmap) else {
    fputs("Unable to render the SVG app icon.\n", stderr)
    exit(1)
}

NSGraphicsContext.saveGraphicsState()
NSGraphicsContext.current = context
context.imageInterpolation = .high
NSColor.clear.set()
NSRect(x: 0, y: 0, width: 1024, height: 1024).fill(using: .copy)
image.draw(in: NSRect(x: 0, y: 0, width: 1024, height: 1024))
NSGraphicsContext.restoreGraphicsState()

guard let data = bitmap.representation(using: .png, properties: [:]) else {
    fputs("Unable to encode the PNG app icon.\n", stderr)
    exit(1)
}
do {
    try data.write(to: URL(fileURLWithPath: destination))
} catch {
    fputs("Unable to write the app icon: \(error)\n", stderr)
    exit(1)
}
