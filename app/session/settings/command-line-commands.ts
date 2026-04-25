// CommandOptions binds a spacewave CLI command to a specific session
// and socket path. sessionIndex is the /u/:idx value. socketPath, when
// non-empty and different from the shared default, is rendered as an
// explicit --socket-path flag so the user can copy-paste the exact
// connect-only form.
export interface CommandOptions {
  sessionIndex: number
  socketPath: string
}

// SHARED_DEFAULT_SOCKET_PATH is the user-facing socket path the CLI
// picks up with no flags when the desktop app is running. The native
// listener reports an absolute path, so isSharedDefaultSocketPath also
// accepts any resolved home path ending in the same suffix.
export const SHARED_DEFAULT_SOCKET_PATH = '~/.spacewave/spacewave.sock'
const SHARED_DEFAULT_SOCKET_SUFFIX = '/.spacewave/spacewave.sock'

// DEFAULT_SESSION_INDEX matches the CLI's own default for
// --session-index, so session 1 produces the shortest command with no
// explicit index.
export const DEFAULT_SESSION_INDEX = 1

// buildSpacewaveCommand renders a spacewave invocation bound to the
// active session. Omits --session-index when the CLI's default already
// matches; omits --socket-path when the live listener path is empty or
// matches the shared default the CLI resolves with no flags. Explicit
// socket paths are single-quoted for POSIX shell copy-paste safety.
//
// Examples:
//   buildSpacewaveCommand('status', {sessionIndex: 1, socketPath: ''})
//     -> 'spacewave status'
//   buildSpacewaveCommand('status', {sessionIndex: 2, socketPath: ''})
//     -> 'spacewave --session-index 2 status'
//   buildSpacewaveCommand('status', {sessionIndex: 1,
//     socketPath: '/run/custom.sock'})
//     -> "spacewave --socket-path '/run/custom.sock' status"
export function buildSpacewaveCommand(
  subcommand: string,
  opts: CommandOptions,
): string {
  const parts: string[] = ['spacewave']
  if (opts.socketPath && !isSharedDefaultSocketPath(opts.socketPath)) {
    parts.push(`--socket-path ${quotePosixShellArg(opts.socketPath)}`)
  }
  if (opts.sessionIndex && opts.sessionIndex !== DEFAULT_SESSION_INDEX) {
    parts.push(`--session-index ${opts.sessionIndex}`)
  }
  parts.push(subcommand)
  return parts.join(' ')
}

// isSharedDefaultSocketPath reports whether a listener path is the shared
// desktop default, either in user-facing "~/" form or resolved absolute form.
export function isSharedDefaultSocketPath(path: string): boolean {
  return (
    path === SHARED_DEFAULT_SOCKET_PATH ||
    path.endsWith(SHARED_DEFAULT_SOCKET_SUFFIX)
  )
}

// quotePosixShellArg returns a single shell token safe for POSIX shells.
// The CLI setup page presents copy-paste terminal commands for macOS/Linux;
// Windows users can still run the quoted form from Git Bash or WSL.
export function quotePosixShellArg(value: string): string {
  return `'${value.replaceAll("'", "'\"'\"'")}'`
}
