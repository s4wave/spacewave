import Foundation
import SystemConfiguration
import SpacewaveUpdateSupport

// spacewave-helper-privileged: SMJobBless-launched root tool that performs
// the /Applications/Spacewave.app swap the interim AEWP shim used to do.
//
// Invocation:
//   spacewave-helper-privileged --swap --current <path> --staged <path>
//
// Runs as root (launchd HelperTool at /Library/PrivilegedHelperTools/
// us.aperture.spacewave.helper). The main app blesses + spawns this binary
// via SMJobBless; we read argv directly rather than doing XPC so the v1
// surface stays small. Atomic rename + LaunchServices register + exit.

enum ExitCode: Int32 {
    case ok = 0
    case usage = 64
    case validation = 65
    case currentMove = 71
    case stagedMove = 72
}

func fail(_ code: ExitCode, _ msg: String) -> Never {
    fputs("spacewave-helper-privileged: \(msg)\n", stderr)
    exit(code.rawValue)
}

struct Args {
    var swap = false
    var currentPath: String?
    var stagedPath: String?
}

func parseArgs() -> Args {
    var args = Args()
    let argv = CommandLine.arguments
    var i = 1
    while i < argv.count {
        switch argv[i] {
        case "--swap":
            args.swap = true
        case "--current":
            i += 1
            if i >= argv.count { fail(.usage, "--current requires a path") }
            args.currentPath = argv[i]
        case "--staged":
            i += 1
            if i >= argv.count { fail(.usage, "--staged requires a path") }
            args.stagedPath = argv[i]
        default:
            fail(.usage, "unknown argument: \(argv[i])")
        }
        i += 1
    }
    return args
}

let parsed = parseArgs()
guard parsed.swap,
      let currentPath = parsed.currentPath,
      let stagedPath = parsed.stagedPath else {
    fail(.usage, "usage: spacewave-helper-privileged --swap --current <path> --staged <path>")
}

let validatedPaths: ValidatedUpdatePaths
do {
    let homeDirectory = try currentConsoleUserHomeDirectory()
    validatedPaths = try validatePrivilegedUpdatePaths(
        currentPath: currentPath,
        stagedPath: stagedPath,
        homeDirectory: homeDirectory
    )
} catch let error as BundleSwapError {
    switch error {
    case .invalidCurrentPath(let message), .invalidStagedPath(let message):
        fail(.validation, message)
    case .currentMoveFailed(let message):
        fail(.currentMove, message)
    case .stagedMoveFailed(let message):
        fail(.stagedMove, message)
    }
} catch {
    fail(.validation, "validate updater paths: \(error)")
}

do {
    try performBundleSwap(
        currentPath: validatedPaths.currentPath,
        stagedPath: validatedPaths.stagedPath
    )
} catch let error as BundleSwapError {
    switch error {
    case .invalidCurrentPath(let message), .invalidStagedPath(let message):
        fail(.validation, message)
    case .currentMoveFailed(let message):
        fail(.currentMove, message)
    case .stagedMoveFailed(let message):
        fail(.stagedMove, message)
    }
} catch {
    fail(.stagedMove, "perform privileged swap: \(error)")
}

// Best-effort LaunchServices register so Finder picks up the new bundle
// version without a full rescan. Failure here is non-fatal because the
// bundle swap itself already succeeded.
let lsregister = "/System/Library/Frameworks/CoreServices.framework/Versions/A/Frameworks/LaunchServices.framework/Versions/A/Support/lsregister"
let task = Process()
task.launchPath = lsregister
task.arguments = ["-f", validatedPaths.currentPath]
do {
    try task.run()
    task.waitUntilExit()
} catch {
    fputs("spacewave-helper-privileged: lsregister failed: \(error)\n", stderr)
}

exit(ExitCode.ok.rawValue)

private func currentConsoleUserHomeDirectory() throws -> String {
    guard
        let cfUser = SCDynamicStoreCopyConsoleUser(nil, nil, nil),
        let user = cfUser as String?,
        !user.isEmpty,
        user != "loginwindow",
        let homeURL = FileManager.default.homeDirectory(forUser: user)
    else {
        throw BundleSwapError.invalidStagedPath(
            "unable to resolve logged-in user home directory"
        )
    }
    return homeURL.path
}
