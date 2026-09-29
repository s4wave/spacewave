import { spawn, execFile } from "node:child_process";
import { copyFile, lstat, mkdir, mkdtemp, chmod } from "node:fs/promises";
import path from "node:path";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

/** installedAppVersion reads the short version of an installed app bundle. */
export async function installedAppVersion(
  installedAppDir: string,
): Promise<string> {
  const { stdout } = await execFileAsync("plutil", [
    "-extract",
    "CFBundleShortVersionString",
    "raw",
    "-o",
    "-",
    path.join(installedAppDir, "Contents", "Info.plist"),
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
