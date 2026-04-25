import Cocoa
import Security
import ServiceManagement
import SpacewaveUpdateSupport

// UpdateManager handles entrypoint self-update: wait for PID exit,
// swap .app bundles, relaunch.
class UpdateManager {
    let currentPath: String
    let stagedPath: String
    let pid: Int32

    // Label must match the privileged tool's Launchd.plist Label and the
    // key under =SMPrivilegedExecutables= in the main app's Info.plist.
    private let helperLabel = "us.aperture.spacewave.helper"
    private let helperInstalledPath = "/Library/PrivilegedHelperTools/us.aperture.spacewave.helper"

    init(currentPath: String, stagedPath: String, pid: Int32) {
        self.currentPath = currentPath
        self.stagedPath = stagedPath
        self.pid = pid
    }

    func execute() throws {
        waitForProcessExit(pid)

        let fm = FileManager.default
        let backupPath = currentPath + ".old"

        // User-writable parent (typical ~/Applications install) -> in-process
        // swap. Otherwise escalate through SMJobBless.
        let needsElevation = !fm.isWritableFile(atPath: (currentPath as NSString).deletingLastPathComponent)
        if needsElevation {
            try executeElevated()
        } else {
            try executeNormal(fm: fm, backupPath: backupPath)
        }

        let url = URL(fileURLWithPath: currentPath)
        let config = NSWorkspace.OpenConfiguration()
        NSWorkspace.shared.openApplication(at: url, configuration: config) { _, error in
            if let error = error {
                fputs("relaunch error: \(error)\n", stderr)
            }
        }
    }

    private func executeNormal(fm: FileManager, backupPath: String) throws {
        _ = backupPath
        try performBundleSwap(
            fileManager: fm,
            currentPath: currentPath,
            stagedPath: stagedPath
        )
    }

    // executeElevated uses SMJobBless to install a short-lived privileged
    // helper tool under /Library/PrivilegedHelperTools/ and then invokes it
    // directly to perform the atomic bundle swap. The helper is embedded in
    // the .app at Contents/Library/LaunchServices/us.aperture.spacewave.helper
    // and carries the SMAuthorizedClients requirement that identifies this
    // main app by signature. Apple's Gatekeeper cross-references that
    // requirement against the caller's signature before launching the tool
    // as root, so a malicious process impersonating the main app cannot
    // bless the helper.
    private func executeElevated() throws {
        // Acquire admin rights once. Apple shows a single authentication
        // prompt here; the returned AuthorizationRef is valid for the
        // SMJobBless call below, which does not prompt again for the same
        // right within the process lifetime.
        var authRef: AuthorizationRef?
        let createStatus = AuthorizationCreate(nil, nil, [], &authRef)
        guard createStatus == errAuthorizationSuccess, let auth = authRef else {
            throw UpdateError.authorizationFailed
        }
        defer { AuthorizationFree(auth, []) }

        // kSMRightBlessPrivilegedHelper is a String constant; the
        // AuthorizationItem.name field wants a C string whose lifetime
        // extends across the AuthorizationCopyRights call, so hold the
        // cstring in a withCString closure for the duration.
        let copyStatus: OSStatus = kSMRightBlessPrivilegedHelper.withCString { namePtr in
            var item = AuthorizationItem(
                name: namePtr,
                valueLength: 0,
                value: nil,
                flags: 0
            )
            return withUnsafeMutablePointer(to: &item) { itemPtr in
                var rights = AuthorizationRights(count: 1, items: itemPtr)
                let flags: AuthorizationFlags = [.interactionAllowed, .preAuthorize, .extendRights]
                return AuthorizationCopyRights(auth, &rights, nil, flags, nil)
            }
        }
        guard copyStatus == errAuthorizationSuccess else {
            throw UpdateError.authorizationDenied
        }

        // Bless the embedded privileged tool. SMJobBless validates the
        // SMAuthorizedClients requirement in the tool's Info.plist against
        // this process's signature, and the SMPrivilegedExecutables entry
        // in this process's Info.plist against the tool's signature.
        // Mismatch on either side fails here with a CFError.
        var blessError: Unmanaged<CFError>?
        let blessed = SMJobBless(kSMDomainSystemLaunchd, helperLabel as CFString, auth, &blessError)
        guard blessed else {
            let err = blessError?.takeRetainedValue()
            throw UpdateError.blessFailed(err.map { "\($0)" } ?? "unknown")
        }

        // Invoke the blessed tool directly. The swap + LaunchServices
        // register + backup cleanup all happen inside the tool; we only
        // observe the exit status.
        let task = Process()
        task.launchPath = helperInstalledPath
        task.arguments = [
            "--swap",
            "--current", currentPath,
            "--staged", stagedPath,
        ]
        do {
            try task.run()
        } catch {
            throw UpdateError.helperLaunchFailed("\(error)")
        }
        task.waitUntilExit()
        guard task.terminationStatus == 0 else {
            throw UpdateError.helperExited(Int(task.terminationStatus))
        }
    }

    private func waitForProcessExit(_ pid: Int32) {
        // Poll for process exit. Short-lived helper, polling is acceptable here.
        while kill(pid, 0) == 0 {
            usleep(100_000) // 100ms
        }
    }
}

enum UpdateError: Error {
    case authorizationFailed
    case authorizationDenied
    case blessFailed(String)
    case helperLaunchFailed(String)
    case helperExited(Int)
}
