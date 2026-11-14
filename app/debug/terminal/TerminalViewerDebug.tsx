import {
  TerminalSessionState,
  TerminalTargetKind,
  type Terminal,
} from '@s4wave/sdk/terminal/terminal.pb.js'
import { useParams } from '@s4wave/web/router/router.js'

import {
  TerminalView,
  type TerminalViewState,
} from '@s4wave/app/terminal/TerminalView.js'

// session is an active shell on a Device, with every fact Details lists.
const session: Terminal = {
  name: 'forge-a shell',
  state: TerminalSessionState.ACTIVE,
  targetKind: TerminalTargetKind.DEVICE,
  deviceObjectKey: 'devices/forge-a',
  devicePeerId: '12D3KooWQf8bJ3rXk2p9mVdE7sTn4yHc6LwAZuG5oRiN1xBvKt3e',
  command: '/bin/bash -l',
  cols: 96,
  rows: 28,
  createdAt: new Date(2026, 10, 14, 9, 41),
}

// noop stands in for a Retry press.
const noop = () => {}

// views are the fixture states, each one production reaches.
const views: Record<string, TerminalViewState> = {
  active: { body: 'screen', session },
  connecting: {
    body: 'screen',
    session: { ...session, state: TerminalSessionState.CONNECTING },
  },
  closed: {
    body: 'screen',
    session: { ...session, state: TerminalSessionState.CLOSED },
  },
  ssh: {
    body: 'screen',
    session: {
      ...session,
      name: 'prod shell',
      targetKind: TerminalTargetKind.SSH_HOST,
      deviceObjectKey: '',
      sshHostObjectKey: 'hosts/prod',
    },
  },
  details: { body: 'screen', session },
  'state-waiting': { body: 'screen' },
  'watch-failed': {
    body: 'screen',
    notice: {
      state: 'failed',
      message: 'Could not read the session state: stream reset',
      retry: noop,
    },
  },
  waiting: {
    body: 'status',
    status: { state: 'waiting', what: 'the terminal', rows: 0 },
  },
  'read-failed': {
    body: 'status',
    status: {
      state: 'failed',
      message: 'Could not open the terminal: stream closed',
      retry: noop,
    },
  },
}

// screenLines are a shell's last output, standing in for xterm.
const screenLines = [
  'forge-a:~$ uptime',
  ' 09:52:13 up 12 days,  3:04,  1 user,  load average: 0.42, 0.38, 0.31',
  'forge-a:~$ ls repos',
  'glados  spacewave  starpc',
  'forge-a:~$ ',
]

/**
 * TerminalViewerDebug renders the Terminal viewer in one fixture state, with
 * the viewport as the pane, for the viewer sweep: active, connecting, closed,
 * ssh (on an SSH host), details (the Details popover open), state-waiting
 * (the screen before the session state arrives), watch-failed (the state
 * watch's notice above the screen), waiting (the handle loading) and
 * read-failed (the handle failed, with Retry). An unknown or missing state
 * shows active.
 */
export function TerminalViewerDebug() {
  // Read the fixture state from the route.
  const { state = 'active' } = useParams()
  const view = views[state] ?? views.active

  // The screen is a static log in the terminal's colors, wrapping at the
  // pane edge as xterm wraps at its column count.
  return (
    <div className="viewer-pane h-full w-full">
      <TerminalView view={view} detailsOpen={state === 'details'}>
        <div
          role="log"
          aria-label="Terminal screen"
          className="bg-background-dark text-foreground ps-viewer-pad pe-viewer-pad @pane-roomy:ps-viewer-pad-roomy @pane-roomy:pe-viewer-pad-roomy min-h-0 flex-1 overflow-hidden py-1 font-mono text-xs break-all whitespace-pre-wrap"
        >
          {screenLines.join('\n')}
        </div>
      </TerminalView>
    </div>
  )
}
