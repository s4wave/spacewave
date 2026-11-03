import { useEffect, useRef, useState } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'

import { terminalStatusToLoadingView } from '@s4wave/app/loading/status/terminal.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { v86SerialChannelName, type SerialFrame } from './serial-channel.js'

import * as stylex from '@stylexjs/stylex'

import {
  colors as themeColors,
  layout as themeLayout,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    flex: 1,
    overflow: 'hidden',
    backgroundColor: themeColors['--color-zinc-950'],
    position: 'relative',
    minHeight: 0,
  },
  rule2: { display: 'none' },
  rule3: { overflow: 'hidden', height: '100%', width: '100%' },
  rule4: {
    inset: 0,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-dark']} 95%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    position: 'absolute',
  },
})

const stylexStyles = stylex.create({
  rule1: { maxWidth: 'var(--container-sm)', width: '100%' },
})

// VmSerialConsoleProps configures the serial console of one VM.
export interface VmSerialConsoleProps {
  // objectKey is the VmV86 whose serial port the console attaches to.
  objectKey: string
  // booting covers the console with a boot card until the guest writes.
  booting: boolean
  // hidden keeps the console mounted, with its scrollback, but not shown.
  hidden?: boolean
}

// VmSerialConsole is an xterm terminal on the VM's first serial port.
export function VmSerialConsole({
  objectKey,
  booting,
  hidden,
}: VmSerialConsoleProps) {
  const hostRef = useRef<HTMLDivElement | null>(null)
  const [hasOutput, setHasOutput] = useState(false)

  useEffect(() => {
    // Require the mounted host and VM identity before opening the serial console.
    const host = hostRef.current
    if (!host || !objectKey) return

    // Open the terminal and fit it to the host whenever the host resizes,
    // including when the console is shown again.
    const term = new Terminal({
      convertEol: true,
      cursorBlink: true,
      fontSize: 13,
      theme: { background: '#09090b', foreground: '#e5e7eb' },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(host)
    const resize = new ResizeObserver(() => {
      if (host.clientWidth && host.clientHeight) fit.fit()
    })
    resize.observe(host)

    // Relay serial bytes in both directions.
    const channel = new BroadcastChannel(v86SerialChannelName(objectKey))
    channel.onmessage = (ev: MessageEvent<SerialFrame>) => {
      const frame = ev.data
      if (frame?.dir !== 'out' || !frame.data) return
      term.write(frame.data)
      setHasOutput(true)
    }
    const input = term.onData((text) => {
      if (text) channel.postMessage({ dir: 'in', text } satisfies SerialFrame)
    })

    return () => {
      resize.disconnect()
      input.dispose()
      channel.close()
      term.dispose()
    }
  }, [objectKey])

  return (
    <div sx={[stylexStyles2.rule1, !!hidden && stylexStyles2.rule2]}>
      <div ref={hostRef} sx={stylexStyles2.rule3} />
      {booting && !hasOutput && (
        <div sx={stylexStyles2.rule4} role="status" aria-live="polite">
          <LoadingCard
            view={terminalStatusToLoadingView(
              { kind: 'connecting' },
              {
                session: 'console',
                connectingTitle: 'Booting virtual machine',
                connectingDetail:
                  'Waiting for the guest to write to its console.',
              },
            )}
            sx={stylexStyles.rule1}
          />
        </div>
      )}
    </div>
  )
}
