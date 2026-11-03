import { CopyButton } from '@s4wave/web/ui/CopyButton.js'

import type { DownloadEntry } from './manifest.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    marginLeft: `calc(${themeLayout['--spacing']} * 4)`,
    listStyleType: 'decimal',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule5: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
})

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
    <div sx={stylexStyles.rule1}>
      <p sx={stylexStyles.rule2}>
        One-line install for {entry.osLabel} ({entry.archLabel}). Extracts the
        spacewave binary into /usr/local/bin.
      </p>
      <CopyableCommand command={command} />
      <p sx={stylexStyles.rule2}>
        Run <code sx={stylexStyles.rule3}>spacewave status</code> to confirm the
        binary is on your PATH.
      </p>
    </div>
  )
}

function MacOSSnippet({ entry }: { entry: DownloadEntry }) {
  return (
    <div sx={stylexStyles.rule1}>
      <p sx={stylexStyles.rule2}>
        macOS ({entry.archLabel}) ships as a signed and notarized zip archive.
      </p>
      <ol sx={stylexStyles2.rule1}>
        <li sx={nativeChildSpacing.s1}>
          Download <code sx={stylexStyles.rule3}>{entry.filename}</code> and
          extract it.
        </li>
        <li sx={nativeChildSpacing.s1}>
          Move <code sx={stylexStyles.rule3}>spacewave</code> somewhere on your
          PATH, such as <code sx={stylexStyles.rule3}>/usr/local/bin</code>.
        </li>
        <li sx={nativeChildSpacing.s1}>
          Open a new terminal and run{' '}
          <code sx={stylexStyles.rule3}>spacewave status</code>.
        </li>
      </ol>
    </div>
  )
}

function WindowsSnippet({ entry }: { entry: DownloadEntry }) {
  return (
    <div sx={stylexStyles.rule1}>
      <p sx={stylexStyles.rule2}>
        Windows ({entry.archLabel}) ships as a portable zip until a packaged
        installer is available.
      </p>
      <ol sx={stylexStyles2.rule1}>
        <li sx={nativeChildSpacing.s1}>
          Download <code sx={stylexStyles.rule3}>{entry.filename}</code> and
          extract it.
        </li>
        <li sx={nativeChildSpacing.s1}>
          Move <code sx={stylexStyles.rule3}>spacewave.exe</code> somewhere on
          your PATH (for example a folder you create at{' '}
          <code sx={stylexStyles.rule3}>%USERPROFILE%\bin</code> and add to
          PATH).
        </li>
        <li sx={nativeChildSpacing.s1}>
          Open a new terminal and run{' '}
          <code sx={stylexStyles.rule3}>spacewave status</code>.
        </li>
      </ol>
    </div>
  )
}

function CopyableCommand({ command }: { command: string }) {
  return (
    <div sx={stylexStyles.rule4}>
      <code sx={stylexStyles.rule5}>{command}</code>
      <CopyButton text={command} label="Copy command" />
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
