import Foundation

public let privilegedAppPath = "/Applications/Spacewave.app"
public let privilegedBundleIdentifier = "us.aperture.spacewave"

public enum BundleSwapError: Error, Equatable {
    case invalidCurrentPath(String)
    case invalidStagedPath(String)
    case currentMoveFailed(String)
    case stagedMoveFailed(String)
}

public struct ValidatedUpdatePaths: Equatable {
    public let currentPath: String
    public let stagedPath: String

    public init(currentPath: String, stagedPath: String) {
        self.currentPath = currentPath
        self.stagedPath = stagedPath
    }
}

public func canonicalizePath(_ path: String) -> String {
    URL(fileURLWithPath: path).standardizedFileURL.resolvingSymlinksInPath().path
}

public func updateStagingRoot(forHomeDirectory homeDirectory: String) -> String {
    canonicalizePath(
        (homeDirectory as NSString).appendingPathComponent(
            "Library/Application Support/Spacewave/updates"
        )
    )
}

public func validatePrivilegedUpdatePaths(
    currentPath: String,
    stagedPath: String,
    homeDirectory: String,
) throws -> ValidatedUpdatePaths {
    let resolvedCurrentPath = canonicalizePath(currentPath)
    if resolvedCurrentPath != privilegedAppPath {
        throw BundleSwapError.invalidCurrentPath(
            "refusing privileged swap for \(resolvedCurrentPath)"
        )
    }

    let resolvedStagedPath = canonicalizePath(stagedPath)
    let stagingRoot = updateStagingRoot(forHomeDirectory: homeDirectory)
    let allowedPrefix = stagingRoot + "/"
    if resolvedStagedPath != stagingRoot &&
        !resolvedStagedPath.hasPrefix(allowedPrefix) {
        throw BundleSwapError.invalidStagedPath(
            "staged app must live under \(stagingRoot)"
        )
    }
    if !resolvedStagedPath.hasSuffix(".app") {
        throw BundleSwapError.invalidStagedPath(
            "staged path must be a .app bundle"
        )
    }
    let bundleID = readBundleIdentifier(appPath: resolvedStagedPath)
    if bundleID != privilegedBundleIdentifier {
        throw BundleSwapError.invalidStagedPath(
            "staged app bundle identifier mismatch"
        )
    }

    return ValidatedUpdatePaths(
        currentPath: resolvedCurrentPath,
        stagedPath: resolvedStagedPath
    )
}

public func performBundleSwap(
    fileManager: FileManager = .default,
    currentPath: String,
    stagedPath: String,
) throws {
    let backupPath = currentPath + ".old"
    try? fileManager.removeItem(atPath: backupPath)

    do {
        try fileManager.moveItem(atPath: currentPath, toPath: backupPath)
    } catch {
        throw BundleSwapError.currentMoveFailed(
            "move \(currentPath) -> \(backupPath): \(error)"
        )
    }

    do {
        try fileManager.moveItem(atPath: stagedPath, toPath: currentPath)
    } catch {
        do {
            try fileManager.moveItem(atPath: backupPath, toPath: currentPath)
        } catch let restoreError {
            throw BundleSwapError.stagedMoveFailed(
                "move \(stagedPath) -> \(currentPath): \(error); restore also failed: \(restoreError)"
            )
        }
        throw BundleSwapError.stagedMoveFailed(
            "move \(stagedPath) -> \(currentPath): \(error); restored backup"
        )
    }

    try? fileManager.removeItem(atPath: backupPath)
}

private func readBundleIdentifier(appPath: String) -> String? {
    let plistPath = (appPath as NSString).appendingPathComponent(
        "Contents/Info.plist"
    )
    guard
        let plist = NSDictionary(contentsOfFile: plistPath),
        let bundleID = plist["CFBundleIdentifier"] as? String
    else {
        return nil
    }
    return bundleID
}
