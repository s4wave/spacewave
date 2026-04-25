import Cocoa

// spacewave-helper: native macOS helper for loading screen and self-update.
// Modes:
//   --loading --icon <path> --pipe-root <dir> --pipe-id <uuid>
//   --update --current <path> --staged <path> --pid <int> --pipe-root <dir> --pipe-id <uuid>

struct Args {
    var loading = false
    var update = false
    var iconPath: String?
    var pipeRoot: String?
    var pipeId: String?
    var currentPath: String?
    var stagedPath: String?
    var pid: Int32 = 0
}

func parseArgs() -> Args {
    var args = Args()
    let argv = CommandLine.arguments
    var i = 1
    while i < argv.count {
        switch argv[i] {
        case "--loading":
            args.loading = true
        case "--update":
            args.update = true
        case "--icon":
            i += 1
            args.iconPath = argv[i]
        case "--pipe-root":
            i += 1
            args.pipeRoot = argv[i]
        case "--pipe-id":
            i += 1
            args.pipeId = argv[i]
        case "--current":
            i += 1
            args.currentPath = argv[i]
        case "--staged":
            i += 1
            args.stagedPath = argv[i]
        case "--pid":
            i += 1
            args.pid = Int32(argv[i]) ?? 0
        default:
            break
        }
        i += 1
    }
    return args
}

let parsedArgs = parseArgs()

// Build pipe path from pipesock conventions: <rootDir>/.pipe-<uuid>
guard let pipeRoot = parsedArgs.pipeRoot, let pipeId = parsedArgs.pipeId else {
    fputs("error: --pipe-root and --pipe-id required\n", stderr)
    exit(1)
}
let pipePath = (pipeRoot as NSString).appendingPathComponent(".pipe-" + pipeId)

if parsedArgs.loading {
    // Loading screen mode.
    let app = NSApplication.shared
    app.setActivationPolicy(.regular)

    let pipe = PipeClient(pipePath: pipePath)
    do {
        try pipe.connect()
    } catch {
        fputs("error: pipe connect failed: \(error)\n", stderr)
        exit(1)
    }

    // Send ready event.
    do {
        try pipe.sendEvent(HelperEvent(type_: .ready))
    } catch {
        fputs("error: send ready failed: \(error)\n", stderr)
        exit(1)
    }

    let loadingWindow = LoadingWindow(iconPath: parsedArgs.iconPath)

    // Wire pipe messages to window.
    pipe.onMessage = { msg in
        loadingWindow.handleMessage(msg)
    }

    // Wire window events back to pipe.
    loadingWindow.onRetry = {
        try? pipe.sendEvent(HelperEvent(type_: .retry))
    }
    loadingWindow.onCancel = {
        try? pipe.sendEvent(HelperEvent(type_: .cancel))
        NSApp.terminate(nil)
    }

    loadingWindow.show()
    pipe.startReadLoop()
    app.run()

} else if parsedArgs.update {
    guard let currentPath = parsedArgs.currentPath,
          let stagedPath = parsedArgs.stagedPath,
          parsedArgs.pid > 0 else {
        fputs("error: --update requires --current, --staged, --pid\n", stderr)
        exit(1)
    }

    // Connect pipe for status reporting.
    let pipe = PipeClient(pipePath: pipePath)
    do {
        try pipe.connect()
        try pipe.sendEvent(HelperEvent(type_: .ready))
    } catch {
        fputs("warning: pipe connect failed, proceeding without IPC: \(error)\n", stderr)
    }

    let updater = UpdateManager(
        currentPath: currentPath,
        stagedPath: stagedPath,
        pid: parsedArgs.pid
    )

    do {
        try updater.execute()
    } catch {
        fputs("error: update failed: \(error)\n", stderr)
        exit(1)
    }

    pipe.close()

} else {
    fputs("error: specify --loading or --update\n", stderr)
    exit(1)
}
