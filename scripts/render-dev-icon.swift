import AppKit
import ImageIO

// Native rendering keeps the build independent of Python imaging packages.
struct Request: Decodable {
    let source: String
    let outputs: [String: String]
}

enum IconError: Error { case invalid(String) }

func openSource(_ path: String) throws -> CGImageSource {
    guard let source = CGImageSourceCreateWithURL(URL(fileURLWithPath: path) as CFURL, nil),
          let type = CGImageSourceGetType(source) as String?,
          ["public.png", "com.microsoft.ico", "com.apple.icns"].contains(type) else {
        throw IconError.invalid("Supported inputs are PNG, ICO and ICNS.")
    }
    if type == "public.png" && CGImageSourceGetCount(source) != 1 {
        throw IconError.invalid("Animated PNG inputs are not supported.")
    }
    return source
}

func frames(_ source: CGImageSource) throws -> [Int: Int] {
    var result: [Int: Int] = [:]
    for index in 0..<CGImageSourceGetCount(source) {
        guard let properties = CGImageSourceCopyPropertiesAtIndex(source, index, nil) as? [String: Any],
              let width = properties[kCGImagePropertyPixelWidth as String] as? Int,
              let height = properties[kCGImagePropertyPixelHeight as String] as? Int,
              width == height, (16...4096).contains(width) else {
            throw IconError.invalid("Artwork must be square and between 16 and 4096 pixels.")
        }
        if result[width] == nil { result[width] = index }
    }
    guard !result.isEmpty else { throw IconError.invalid("No usable icon representations found.") }
    return result
}

func render(_ source: CGImageSource, _ representations: [Int: Int], _ edge: Int, _ path: String) throws {
    let choice = representations.keys.sorted().first(where: { $0 >= edge }) ?? representations.keys.max()!
    guard let image = CGImageSourceCreateImageAtIndex(source, representations[choice]!, nil),
          let bitmap = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: edge, pixelsHigh: edge,
              bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
              colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0),
          let context = NSGraphicsContext(bitmapImageRep: bitmap) else {
        throw IconError.invalid("Unable to decode or render the source artwork.")
    }
    NSGraphicsContext.saveGraphicsState()
    defer { NSGraphicsContext.restoreGraphicsState() }
    NSGraphicsContext.current = context
    context.imageInterpolation = .high
    let canvas = NSRect(x: 0, y: 0, width: edge, height: edge)
    NSColor.clear.set()
    canvas.fill(using: .copy)
    NSImage(cgImage: image, size: canvas.size).draw(in: canvas)

    let size = CGFloat(edge)
    let font = NSFont.systemFont(ofSize: max(edge < 20 ? 6 : 7, (size * 0.21).rounded()), weight: .bold)
    let attributes: [NSAttributedString.Key: Any] = [.font: font, .foregroundColor: NSColor.white]
    let text = "Dev" as NSString
    let textSize = text.size(withAttributes: attributes)
    let padX = max(1, (size * 0.045).rounded())
    let padY = max(1, (size * 0.03).rounded())
    let margin = max(1, (size * 0.035).rounded())
    let rect = NSRect(x: size - margin - ceil(textSize.width) - padX * 2, y: margin,
                      width: ceil(textSize.width) + padX * 2, height: ceil(textSize.height) + padY * 2)
    guard rect.minX >= 0, rect.maxY <= size else { throw IconError.invalid("Dev badge does not fit at \(edge)px.") }
    let border = max(1, (size * 0.006).rounded())
    let shape = NSBezierPath(roundedRect: rect.insetBy(dx: border / 2, dy: border / 2),
                             xRadius: rect.height * 0.25, yRadius: rect.height * 0.25)
    NSColor(calibratedRed: 23/255, green: 25/255, blue: 31/255, alpha: 1).setFill()
    shape.fill()
    NSColor.white.withAlphaComponent(0.96).setStroke()
    shape.lineWidth = border
    shape.stroke()
    text.draw(at: NSPoint(x: rect.minX + padX, y: rect.minY + padY), withAttributes: attributes)
    guard let data = bitmap.representation(using: .png, properties: [:]) else {
        throw IconError.invalid("Unable to encode PNG artwork.")
    }
    try data.write(to: URL(fileURLWithPath: path))
}

do {
    if CommandLine.arguments.count == 3 && CommandLine.arguments[1] == "--inspect" {
        let source = try openSource(CommandLine.arguments[2])
        let sizes = try frames(source).keys.sorted()
        let data = try JSONEncoder().encode(sizes)
        print(String(decoding: data, as: UTF8.self))
    } else if CommandLine.arguments.count == 1 {
        let request = try JSONDecoder().decode(Request.self, from: FileHandle.standardInput.readDataToEndOfFile())
        let source = try openSource(request.source)
        let representations = try frames(source)
        for (size, path) in request.outputs {
            guard let edge = Int(size), (16...4096).contains(edge) else {
                throw IconError.invalid("Invalid output size.")
            }
            try render(source, representations, edge, path)
        }
    } else {
        throw IconError.invalid("Usage: render-dev-icon.swift [--inspect source]")
    }
} catch {
    fputs("dev-icon renderer: \(error)\n", stderr)
    exit(1)
}
