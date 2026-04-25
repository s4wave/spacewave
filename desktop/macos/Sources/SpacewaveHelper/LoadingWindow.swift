import Cocoa

// LoadingWindow displays a native macOS loading splash with an icon, title,
// progress bar, and status text. Mirrors the glassy, chromeless aesthetic of
// the Spacewave web UI: translucent vibrancy background, hidden title bar,
// centered content, quiet typography. Communicates retry/cancel back via
// callbacks.
class LoadingWindow: NSObject, NSWindowDelegate {
    let window: NSWindow
    private let imageView: NSImageView
    private let titleLabel: NSTextField
    private let progressBar: NSProgressIndicator
    private let statusLabel: NSTextField
    private let retryButton: NSButton

    var onRetry: (() -> Void)?
    var onCancel: (() -> Void)?

    init(iconPath: String?) {
        let width: CGFloat = 400
        let height: CGFloat = 300
        let rect = NSRect(x: 0, y: 0, width: width, height: height)

        window = NSWindow(
            contentRect: rect,
            styleMask: [.titled, .closable, .fullSizeContentView],
            backing: .buffered,
            defer: false
        )
        window.title = "Spacewave"
        window.titlebarAppearsTransparent = true
        window.titleVisibility = .hidden
        window.isMovableByWindowBackground = true
        window.isReleasedWhenClosed = false
        window.backgroundColor = .clear
        window.isOpaque = false
        window.center()

        // Glass background via vibrancy.
        let blur = NSVisualEffectView(frame: rect)
        blur.material = .hudWindow
        blur.blendingMode = .behindWindow
        blur.state = .active
        blur.autoresizingMask = [.width, .height]
        window.contentView = blur
        let content = blur

        // Content stack is vertically centered in the window. Positions are
        // computed so the visible stack (icon + title + bar + status) is
        // balanced between top and bottom, ignoring the retry button which
        // only appears on error.
        let iconSize: CGFloat = 96
        imageView = NSImageView(frame: NSRect(
            x: (width - iconSize) / 2,
            y: 149,
            width: iconSize,
            height: iconSize
        ))
        imageView.imageScaling = .scaleProportionallyUpOrDown
        if let path = iconPath, let image = NSImage(contentsOfFile: path) {
            imageView.image = image
        }
        content.addSubview(imageView)

        // Title: "Spacewave" in semibold, primary label color.
        titleLabel = NSTextField(frame: NSRect(x: 30, y: 107, width: width - 60, height: 24))
        titleLabel.isEditable = false
        titleLabel.isBordered = false
        titleLabel.isSelectable = false
        titleLabel.drawsBackground = false
        titleLabel.backgroundColor = .clear
        titleLabel.alignment = .center
        titleLabel.font = NSFont.systemFont(ofSize: 17, weight: .semibold)
        titleLabel.textColor = .labelColor
        titleLabel.stringValue = "Spacewave"
        content.addSubview(titleLabel)

        // Progress bar: thinner, narrower than the old 300x20.
        let barWidth: CGFloat = 240
        progressBar = NSProgressIndicator(frame: NSRect(
            x: (width - barWidth) / 2,
            y: 85,
            width: barWidth,
            height: 8
        ))
        progressBar.style = .bar
        progressBar.isIndeterminate = true
        progressBar.controlSize = .small
        progressBar.startAnimation(nil)
        content.addSubview(progressBar)

        // Status: muted secondary label color, smaller than the title.
        statusLabel = NSTextField(frame: NSRect(x: 30, y: 55, width: width - 60, height: 18))
        statusLabel.isEditable = false
        statusLabel.isBordered = false
        statusLabel.isSelectable = false
        statusLabel.drawsBackground = false
        statusLabel.backgroundColor = .clear
        statusLabel.alignment = .center
        statusLabel.font = NSFont.systemFont(ofSize: 12)
        statusLabel.textColor = .secondaryLabelColor
        statusLabel.stringValue = "Connecting..."
        content.addSubview(statusLabel)

        // Retry: hidden by default, shown on retryable errors.
        let btnWidth: CGFloat = 120
        retryButton = NSButton(frame: NSRect(
            x: (width - btnWidth) / 2,
            y: 14,
            width: btnWidth,
            height: 28
        ))
        retryButton.title = "Retry"
        retryButton.bezelStyle = .rounded
        retryButton.controlSize = .regular
        retryButton.isHidden = true
        content.addSubview(retryButton)

        super.init()
        window.delegate = self
        retryButton.target = self
        retryButton.action = #selector(retryClicked)
    }

    func show() {
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    func handleMessage(_ msg: HelperMessage) {
        if let progress = msg.progress {
            retryButton.isHidden = true
            statusLabel.textColor = .secondaryLabelColor
            if progress.fraction < 0 {
                progressBar.isIndeterminate = true
                progressBar.startAnimation(nil)
            } else {
                progressBar.isIndeterminate = false
                progressBar.doubleValue = Double(progress.fraction) * 100
            }
            if !progress.text.isEmpty {
                statusLabel.stringValue = progress.text
            }
        }

        if let status = msg.status {
            statusLabel.textColor = .secondaryLabelColor
            statusLabel.stringValue = status.text
        }

        if msg.dismiss {
            NSApp.terminate(nil)
        }

        if let error = msg.error {
            statusLabel.textColor = .systemRed
            statusLabel.stringValue = error.message
            progressBar.stopAnimation(nil)
            if error.retryable {
                retryButton.isHidden = false
            }
        }
    }

    @objc func retryClicked() {
        retryButton.isHidden = true
        statusLabel.textColor = .secondaryLabelColor
        statusLabel.stringValue = "Retrying..."
        progressBar.isIndeterminate = true
        progressBar.startAnimation(nil)
        onRetry?()
    }

    func windowShouldClose(_ sender: NSWindow) -> Bool {
        onCancel?()
        return false
    }
}
