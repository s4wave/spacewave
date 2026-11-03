import { pushable, type Pushable } from 'it-pushable'
import { useCallback, useMemo, useRef, useState } from 'react'
import { LuAppWindow, LuMonitor } from '@s4wave/web/images/icons.js'

import { WasiVmTypeID, type WasiVmHandle } from '@s4wave/sdk/vm/wasivm.js'
import {
  WasiVmWindowPlace,
  type WasiVmWindow,
  type WasiVmWindowInput,
} from '@s4wave/sdk/vm/wasivm.pb.js'
import { createObjectLinkAddTabRequest } from '@s4wave/web/object/ObjectLink.js'
import { useTabContext } from '@s4wave/web/object/TabContext.js'
import { localNavigation } from '@s4wave/web/router/HistoryRouter.js'
import { useNavigate, usePath } from '@s4wave/web/router/router.js'
import { Button } from '@s4wave/web/ui/button.js'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'

import {
  WasiVmDisplay,
  type WasiVmDisplayConnector,
  type WasiVmDisplayViews,
  type WasiVmPaneSize,
} from './WasiVmDisplay.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    display: { default: 'none', '@container (min-width: 28rem)': 'inline' },
  },
  rule2: {
    flex: 1,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule3: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: {
    padding: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    flex: 1,
    display: 'flex',
    justifyContent: 'center',
    minHeight: 0,
    overflowY: 'auto',
  },
  rule5: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    display: 'flex',
    flexDirection: 'column',
    height: 'fit-content',
    maxWidth: 'var(--container-md)',
    width: '100%',
  },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule7: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule8: { flex: 1, minWidth: 0, },
  rule9: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-base)',
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule10: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule11: {
    gap: themeLayout['--spacing'],
    display: 'flex',
    flexDirection: 'column',
  },
  rule12: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule13: {
    gap: `calc(${themeLayout['--spacing']} * .5)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule14: {
    '--tw-duration': '.15s',
    borderRadius: themeRadii['--radius-md'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'left',
    transitionDuration: '.15s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
})

const stylexStyles2 = stylex.create({
  rule1: { minWidth: `calc(${themeLayout['--spacing']} * 45)` },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    color: themeColors['--color-foreground-alt'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

// WasiVmWindowRoute names one window of one run of a VM.
export interface WasiVmWindowRoute {
  run: bigint
  id: number
}

// windowPathPattern matches a window's viewer path, window/<run>/<id>.
const windowPathPattern = /^\/?window\/(\d+)\/(\d+)\/?$/

// parseWindowPath reads the window a viewer path names, or null when it
// names the VM.
export function parseWindowPath(path: string): WasiVmWindowRoute | null {
  const m = windowPathPattern.exec(path)
  return m ? { run: BigInt(m[1]), id: Number(m[2]) } : null
}

// formatWindowPath returns the viewer path of a window.
export function formatWindowPath(route: WasiVmWindowRoute): string {
  return `window/${route.run}/${route.id}`
}

// windowLabel names a window by its title, else its app id, else its id.
export function windowLabel(w: WasiVmWindow): string {
  return w.title || w.appId || `Window ${w.id ?? 0}`
}

// WasiVmWindowView is the window a VM viewer's path names, with the actions
// that move between the VM's view and its windows.
export interface WasiVmWindowView {
  // route is the window the path names, or null for the VM's view.
  route: WasiVmWindowRoute | null
  // shownWindow is the named window while this run lists it.
  shownWindow?: WasiVmWindow
  // showDesktop returns the viewer to the VM's view.
  showDesktop: () => void
  // showWindow shows a window of this run in this viewer.
  showWindow: (id: number) => void
  // openWindow opens a window of this run in a tab of its own, or in this
  // viewer outside a tab host.
  openWindow: (id: number) => void
}

// useWasiVmWindowRoute reads the window the viewer path names among the
// windows of run runGeneration of the VM objectKey. Its paths stay inside the
// viewer: the host resolves them against the VM's object path.
export function useWasiVmWindowRoute(
  objectKey: string,
  runGeneration: bigint,
  windows: WasiVmWindow[] | undefined,
): WasiVmWindowView {
  const route = parseWindowPath(usePath())
  const navigate = useNavigate()
  const tabContext = useTabContext()
  const shownWindow =
    route?.run === runGeneration
      ? windows?.find((w) => w.id === route.id)
      : undefined

  const showDesktop = useCallback(
    () => navigate(localNavigation({ path: '/' })),
    [navigate],
  )
  const showWindow = useCallback(
    (id: number) =>
      navigate(
        localNavigation({
          path: '/' + formatWindowPath({ run: runGeneration, id }),
        }),
      ),
    [navigate, runGeneration],
  )
  const openWindow = (id: number) => {
    if (!tabContext) {
      showWindow(id)
      return
    }
    const w = windows?.find((w) => w.id === id)
    void tabContext
      .addTab(
        createObjectLinkAddTabRequest({
          objectKey,
          objectType: WasiVmTypeID,
          label: w ? windowLabel(w) : undefined,
          path: formatWindowPath({ run: runGeneration, id }),
          currentTabId: tabContext.tabId,
        }),
      )
      .catch((err) => console.warn('WasiVmWindow: failed to add tab:', err))
  }
  return { route, shownWindow, showDesktop, showWindow, openWindow }
}

// WasiVmWindowsMenuProps configures the windows menu of a running VM.
interface WasiVmWindowsMenuProps {
  windows: WasiVmWindow[]
  onOpen: (id: number) => void
}

// WasiVmWindowsMenu lists the windows of a VM's Wayland display, each opening
// in a tab of its own. Its label collapses to the icon in a narrow header.
export function WasiVmWindowsMenu({ windows, onOpen }: WasiVmWindowsMenuProps) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="xs" aria-label="Windows">
          <LuAppWindow />
          <span sx={stylexStyles3.rule1}>Windows</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        variant="compact"
        sx={stylexStyles2.rule1}
      >
        <DropdownMenuLabel>Open in tab</DropdownMenuLabel>
        {windows.length === 0 ? (
          <DropdownMenuItem disabled>No windows are open</DropdownMenuItem>
        ) : (
          windows.map((w) => (
            <DropdownMenuItem key={w.id} onClick={() => onOpen(w.id ?? 0)}>
              <span sx={stylexStyles3.rule2}>{windowLabel(w)}</span>
              {w.place === WasiVmWindowPlace.WasiVmWindowPlace_VIEWER && (
                <span sx={stylexStyles3.rule3}>In a tab</span>
              )}
            </DropdownMenuItem>
          ))
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// WasiVmWindowPickerProps configures the picker of a window tab with no
// window.
interface WasiVmWindowPickerProps {
  name: string
  // earlierRun is set when the tab's window belonged to an earlier run.
  earlierRun: boolean
  // windows is the run's windows, or undefined until the VM lists them.
  windows: WasiVmWindow[] | undefined
  onShowWindow: (id: number) => void
  onShowDesktop: () => void
}

// WasiVmWindowPicker fills a window tab whose window is gone: it names the VM,
// says why the tab is empty, and lists the running VM's windows to show in
// the tab instead. The list follows the VM as windows open and close.
export function WasiVmWindowPicker({
  name,
  earlierRun,
  windows,
  onShowWindow,
  onShowDesktop,
}: WasiVmWindowPickerProps) {
  return (
    <div sx={stylexStyles3.rule4}>
      <section sx={stylexStyles3.rule5}>
        {/* Identity */}
        <div sx={stylexStyles3.rule6}>
          <div sx={stylexStyles3.rule7}>
            <LuAppWindow sx={stylexStyles.rule1} />
          </div>
          <div sx={stylexStyles3.rule8}>
            <h2 sx={stylexStyles3.rule9}>No window in this tab</h2>
            <div sx={stylexStyles3.rule10}>
              {earlierRun
                ? `Its window closed when ${name} last stopped.`
                : `Its window in ${name} closed.`}
            </div>
          </div>
        </div>

        {/* Windows */}
        <div sx={stylexStyles3.rule11}>
          <div sx={stylexStyles3.rule10}>Show a window here</div>
          {windows === undefined ? (
            <div sx={stylexStyles3.rule12}>Reading the VM's windows...</div>
          ) : windows.length === 0 ? (
            <div sx={stylexStyles3.rule12}>
              No windows are open. Open an app on the desktop and it appears
              here.
            </div>
          ) : (
            <ul sx={stylexStyles3.rule13}>
              {windows.map((w) => (
                <li key={w.id}>
                  <button
                    type="button"
                    onClick={() => onShowWindow(w.id ?? 0)}
                    sx={stylexStyles3.rule14}
                  >
                    <LuAppWindow sx={stylexStyles.rule2} />
                    <span sx={stylexStyles3.rule2}>{windowLabel(w)}</span>
                    {w.place === WasiVmWindowPlace.WasiVmWindowPlace_VIEWER && (
                      <span sx={stylexStyles3.rule3}>In another tab</span>
                    )}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        {/* Action */}
        <Button variant="outline" onClick={onShowDesktop}>
          <LuMonitor />
          Show desktop
        </Button>
      </section>
    </div>
  )
}

// WasiVmWindowPaneProps configures the pane showing one window.
interface WasiVmWindowPaneProps {
  handle: WasiVmHandle | undefined
  id: number
  onShowDesktop: () => void
}

// WasiVmWindowPane shows one window of a running VM's Wayland display at the
// pane's size. It opens the window once the pane is measured and resizes it
// as the pane changes; leaving returns the window to the desktop.
export function WasiVmWindowPane({
  handle,
  id,
  onShowDesktop,
}: WasiVmWindowPaneProps) {
  // sizeRef is the pane's last size; windowInput is the open stream's input.
  const sizeRef = useRef<WasiVmPaneSize | null>(null)
  const windowInput = useRef<Pushable<WasiVmWindowInput> | null>(null)
  const [measured, setMeasured] = useState(false)

  // Open the window at the pane's size, passing on the display's input.
  const connectWindow = useMemo<WasiVmDisplayConnector | undefined>(() => {
    if (!handle || !measured) return undefined
    return (input, signal) => {
      const win = pushable<WasiVmWindowInput>({ objectMode: true })
      win.push({
        body: {
          case: 'open',
          value: { id, size: sizeRef.current ?? undefined },
        },
      })
      windowInput.current = win
      void (async () => {
        for await (const ev of input) {
          win.push({ body: { case: 'input', value: ev } })
        }
        win.end()
        if (windowInput.current === win) windowInput.current = null
      })()
      return handle.connectWindow(win, signal)
    }
  }, [handle, measured, id])

  // Resize the open window with the pane.
  const onPaneSize = useCallback((size: WasiVmPaneSize) => {
    sizeRef.current = size
    setMeasured(true)
    windowInput.current?.push({ body: { case: 'resize', value: size } })
  }, [])

  const views = useMemo<WasiVmDisplayViews>(
    () => ({
      waiting: {
        state: 'active',
        title: 'Opening the window',
        detail: 'It appears here once the VM draws it at this size.',
      },
      closed: () => ({
        state: 'error',
        title: 'The window closed',
        detail: 'Its app closed it, or the VM stopped its display.',
        onRetry: onShowDesktop,
        retryLabel: 'Show desktop',
      }),
      failed: (detail, retry) => ({
        state: 'error',
        title: 'The window disconnected',
        detail,
        onRetry: retry,
        onCancel: onShowDesktop,
        cancelLabel: 'Show desktop',
      }),
    }),
    [onShowDesktop],
  )

  return (
    <WasiVmDisplay
      connectDisplay={connectWindow}
      views={views}
      onPaneSize={onPaneSize}
    />
  )
}
