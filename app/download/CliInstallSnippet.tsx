import { CopyButton } from '@s4wave/web/ui/CopyButton.js'

import type { DownloadEntry } from './manifest.js'

interface CliInstallSnippetProps {
  entry: DownloadEntry
}

// CliInstallSnippet renders the per-OS install instructions for a CLI
// archive entry. Linux targets get a curl + tar one-liner; macOS and
// Windows targets get zip-extract instructions. Each snippet has a
// copy button where a one-line install is safe.
export function CliInstallSnippet({ entry }: CliInstallSnippetProps) {
  if (entry.os === 'windows') {
    return <WindowsSnippet entry={entry} />
  }
  if (entry.os === 'macos') {
    return <MacOSSnippet entry={entry} />
  }
  return <UnixSnippet entry={entry} />
}

function UnixSnippet({ entry }: { entry: DownloadEntry }) {
  const command = `curl -fsSL ${entry.url} | tar -xz -C /usr/local/bin spacewave`
  return (
    <div className="flex flex-col gap-2">
      <p className="text-foreground-alt text-xs">
        One-line install for {entry.osLabel} ({entry.archLabel}). Extracts the
        spacewave binary into /usr/local/bin.
      </p>
      <CopyableCommand command={command} />
      <p className="text-foreground-alt text-xs">
        Run <code className="font-mono">spacewave status</code> to confirm the
        binary is on your PATH.
      </p>
    </div>
  )
}

function MacOSSnippet({ entry }: { entry: DownloadEntry }) {
  return (
    <div className="flex flex-col gap-2">
      <p className="text-foreground-alt text-xs">
        macOS ({entry.archLabel}) ships as a signed and notarized zip archive.
      </p>
      <ol className="text-foreground-alt ml-4 list-decimal space-y-1 text-xs">
        <li>
          Download <code className="font-mono">{entry.filename}</code> and
          extract it.
        </li>
        <li>
          Move <code className="font-mono">spacewave-cli</code> somewhere on
          your PATH, such as <code className="font-mono">/usr/local/bin</code>.
        </li>
        <li>
          Open a new terminal and run{' '}
          <code className="font-mono">spacewave status</code>.
        </li>
      </ol>
    </div>
  )
}

function WindowsSnippet({ entry }: { entry: DownloadEntry }) {
  return (
    <div className="flex flex-col gap-2">
      <p className="text-foreground-alt text-xs">
        Windows ({entry.archLabel}) ships as a portable zip until a packaged
        installer is available.
      </p>
      <ol className="text-foreground-alt ml-4 list-decimal space-y-1 text-xs">
        <li>
          Download <code className="font-mono">{entry.filename}</code> and
          extract it.
        </li>
        <li>
          Move <code className="font-mono">spacewave.exe</code> somewhere on
          your PATH (for example a folder you create at{' '}
          <code className="font-mono">%USERPROFILE%\bin</code> and add to PATH).
        </li>
        <li>
          Open a new terminal and run{' '}
          <code className="font-mono">spacewave status</code>.
        </li>
      </ol>
    </div>
  )
}

function CopyableCommand({ command }: { command: string }) {
  return (
    <div className="border-foreground/10 bg-background/40 flex items-center gap-2 rounded-md border px-2.5 py-1.5">
      <code className="text-foreground min-w-0 flex-1 truncate font-mono text-xs">
        {command}
      </code>
      <CopyButton text={command} label="Copy command" />
    </div>
  )
}
