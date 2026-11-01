import { useEffect, useRef, useState } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'

import { terminalStatusToLoadingView } from '@s4wave/app/loading/status/terminal.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { cn } from '@s4wave/web/style/utils.js'

import { v86SerialChannelName, type SerialFrame } from './serial-channel.js'

import * as stylex from '@stylexjs/stylex'

const stylexStyles = stylex.create({
  rule1: { width: '100%', maxWidth: 'var(--container-sm)' },
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
    <div
      className={cn(
        'relative min-h-0 flex-1 overflow-hidden bg-zinc-950 p-2',
        hidden && 'hidden',
      )}
    >
      <div ref={hostRef} className="size-full overflow-hidden" />
      {booting && !hasOutput && (
        <div
          className="bg-background-dark/95 absolute inset-0 flex items-center justify-center p-4"
          role="status"
          aria-live="polite"
        >
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
