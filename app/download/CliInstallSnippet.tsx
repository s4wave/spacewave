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
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    listStyleType: 'decimal',
    marginLeft: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule2: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule3: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    fontFamily: themeFonts['--font-mono'],
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule4: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
    display: 'flex',
  },
  rule5: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    flex: 1,
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    minWidth: 0,
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
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
  },
})
