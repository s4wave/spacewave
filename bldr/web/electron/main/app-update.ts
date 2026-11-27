import { spawn, execFile } from "node:child_process";
import { copyFile, lstat, mkdir, mkdtemp, chmod } from "node:fs/promises";
import path from "node:path";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

/**
 * installedAppVersion reads the version of an installed macOS app bundle or
 * Windows launcher.
 */
export async function installedAppVersion(
  installedApp: string,
): Promise<string> {
  // The Windows launcher has its version in its version resource.
  if (process.platform === "win32") {
    const { stdout } = await execFileAsync(
      "powershell.exe",
      [
        "-NoProfile",
        "-NonInteractive",
        "-Command",
        "(Get-Item -LiteralPath $env:SPACEWAVE_INSTALLED_APP).VersionInfo.ProductVersion",
      ],
      {
        env: { ...process.env, SPACEWAVE_INSTALLED_APP: installedApp },
        windowsHide: true,
      },
    );
    return stdout.trim();
  }

  const { stdout } = await execFileAsync("plutil", [
    "-extract",
    "CFBundleShortVersionString",
    "raw",
    "-o",
    "-",
    path.join(installedApp, "Contents", "Info.plist"),
  ]);
  return stdout.trim();
}

/** startAppBundleUpdate replaces the installed app that opened the desktop. */
export async function startAppBundleUpdate(
  stagedAppDir: string,
  installedAppDir: string,
  stateDir: string,
  pid: number,
): Promise<void> {
  // The desktop runs Electron from its UI artifact, so the installed bundle
  // comes from the app launch that opened it, never from process.execPath.
  if (!path.isAbsolute(installedAppDir) || !installedAppDir.endsWith(".app")) {
    throw new Error("desktop was not opened from an installed .app bundle");
  }
  const installedInfo = await lstat(installedAppDir);
  if (!installedInfo.isDirectory() || installedInfo.isSymbolicLink()) {
    throw new Error("installed app is not a real .app directory");
  }

  // Recheck the artifact after the daemon's verified handoff.
  if (!path.isAbsolute(stagedAppDir) || !stagedAppDir.endsWith(".app")) {
    throw new Error("staged update is not an absolute .app bundle");
  }
  const stagedInfo = await lstat(stagedAppDir);
  if (!stagedInfo.isDirectory() || stagedInfo.isSymbolicLink()) {
    throw new Error("staged update is not a real .app directory");
  }
  await execFileAsync("codesign", [
    "--verify",
    "--deep",
    "--strict",
    stagedAppDir,
  ]);

  // Place the helper outside the bundle it will move, in a unique directory.
  const stagedHelper = path.join(
    stagedAppDir,
    "Contents",
    "MacOS",
    "spacewave-helper",
  );
  const helperInfo = await lstat(stagedHelper);
  if (!helperInfo.isFile() || helperInfo.isSymbolicLink()) {
    throw new Error("staged update helper is not a regular file");
  }
  await mkdir(stateDir, { recursive: true });
  const helperDir = await mkdtemp(path.join(stateDir, "app-update-"));
  const helperPath = path.join(helperDir, "spacewave-helper");
  await copyFile(stagedHelper, helperPath);
  await chmod(helperPath, 0o755);

  // The helper waits for this Electron PID, swaps the app, and relaunches it.
  const child = spawn(
    helperPath,
    [
      "--update",
      "--current",
      installedAppDir,
      "--staged",
      stagedAppDir,
      "--pid",
      String(pid),
      "--pipe-root",
      stateDir,
      "--pipe-id",
      "update",
    ],
    { detached: true, stdio: "ignore" },
  );
  await new Promise<void>((resolve, reject) => {
    child.once("spawn", resolve);
    child.once("error", reject);
  });
  child.unref();
}

/**
 * startLauncherUpdate has the staged Windows launcher replace the installed
 * one. The staged exe waits for this Electron PID, installs itself and its
 * helper, and starts the installed launcher, which reopens the desktop.
 */
export async function startLauncherUpdate(
  stagedExe: string,
  installedLauncher: string,
  pid: number,
): Promise<void> {
  // The installed launcher comes from the launch that opened the desktop.
  if (!isAbsoluteExe(installedLauncher)) {
    throw new Error("desktop was not opened from an installed launcher");
  }
  const installedInfo = await lstat(installedLauncher);
  if (!installedInfo.isFile() || installedInfo.isSymbolicLink()) {
    throw new Error("installed launcher is not a regular file");
  }

  // Recheck the artifact after the daemon's verified handoff.
  if (!isAbsoluteExe(stagedExe)) {
    throw new Error("staged update is not an absolute .exe");
  }
  const stagedInfo = await lstat(stagedExe);
  if (!stagedInfo.isFile() || stagedInfo.isSymbolicLink()) {
    throw new Error("staged update is not a regular file");
  }

  // The staged launcher outlives this process to finish the replacement.
  const child = spawn(stagedExe, ["apply-update", "--pid", String(pid)], {
    detached: true,
    stdio: "ignore",
    windowsHide: true,
  });
  await new Promise<void>((resolve, reject) => {
    child.once("spawn", resolve);
    child.once("error", reject);
  });
  child.unref();
}

// isAbsoluteExe reports whether a path is an absolute Windows executable path.
function isAbsoluteExe(exePath: string): boolean {
  return (
    path.win32.isAbsolute(exePath) && exePath.toLowerCase().endsWith(".exe")
  );
}
