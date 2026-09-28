import { spawn, execFile } from 'node:child_process'
import { copyFile, lstat, mkdir, mkdtemp, chmod } from 'node:fs/promises'
import path from 'node:path'
import { promisify } from 'node:util'

const verifyCodesign = promisify(execFile)

/** startAppBundleUpdate starts the staged app's helper outside both bundles. */
export async function startAppBundleUpdate(
  stagedAppDir: string,
  currentExecutable: string,
  stateDir: string,
  pid: number,
): Promise<void> {
  // Resolve the installed bundle from Electron's own executable identity.
  const macosDir = path.dirname(currentExecutable)
  const contentsDir = path.dirname(macosDir)
  const currentAppDir = path.dirname(contentsDir)
  if (
    path.basename(macosDir) !== 'MacOS' ||
    path.basename(contentsDir) !== 'Contents' ||
    !currentAppDir.endsWith('.app')
  ) {
    throw new Error('Electron is not running from an installed .app bundle')
  }

  // Recheck the artifact after the daemon's verified handoff.
  if (!path.isAbsolute(stagedAppDir) || !stagedAppDir.endsWith('.app')) {
    throw new Error('staged update is not an absolute .app bundle')
  }
  const stagedInfo = await lstat(stagedAppDir)
  if (!stagedInfo.isDirectory() || stagedInfo.isSymbolicLink()) {
    throw new Error('staged update is not a real .app directory')
  }
  await verifyCodesign('codesign', [
    '--verify',
    '--deep',
    '--strict',
    stagedAppDir,
  ])

  // Place the helper outside the bundle it will move, in a unique directory.
  const stagedHelper = path.join(
    stagedAppDir,
    'Contents',
    'MacOS',
    'spacewave-helper',
  )
  const helperInfo = await lstat(stagedHelper)
  if (!helperInfo.isFile() || helperInfo.isSymbolicLink()) {
    throw new Error('staged update helper is not a regular file')
  }
  await mkdir(stateDir, { recursive: true })
  const helperDir = await mkdtemp(path.join(stateDir, 'app-update-'))
  const helperPath = path.join(helperDir, 'spacewave-helper')
  await copyFile(stagedHelper, helperPath)
  await chmod(helperPath, 0o755)

  // The helper waits for this Electron PID, swaps the app, and relaunches it.
  const child = spawn(
    helperPath,
    [
      '--update',
      '--current',
      currentAppDir,
      '--staged',
      stagedAppDir,
      '--pid',
      String(pid),
      '--pipe-root',
      stateDir,
      '--pipe-id',
      'update',
    ],
    { detached: true, stdio: 'ignore' },
  )
  await new Promise<void>((resolve, reject) => {
    child.once('spawn', resolve)
    child.once('error', reject)
  })
  child.unref()
}
