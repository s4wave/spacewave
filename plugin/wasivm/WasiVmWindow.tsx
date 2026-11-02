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


const stylexStyles2 = stylex.create({"rule1":{"minWidth":`calc(${themeLayout["--spacing"]} * 45)`}})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
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
          <span className="hidden @md:inline">Windows</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" variant="compact" sx={stylexStyles2.rule1}>
        <DropdownMenuLabel>Open in tab</DropdownMenuLabel>
        {windows.length === 0 ? (
          <DropdownMenuItem disabled>No windows are open</DropdownMenuItem>
        ) : (
          windows.map((w) => (
            <DropdownMenuItem key={w.id} onClick={() => onOpen(w.id ?? 0)}>
              <span className="min-w-0 flex-1 truncate">{windowLabel(w)}</span>
              {w.place === WasiVmWindowPlace.WasiVmWindowPlace_VIEWER && (
                <span className="text-foreground-alt/60 text-xs">In a tab</span>
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
    <div className="flex min-h-0 flex-1 justify-center overflow-y-auto p-4 sm:p-8">
      <section className="border-foreground/6 bg-background-card/30 flex h-fit w-full max-w-md flex-col gap-4 rounded-lg border p-4 backdrop-blur-sm">
        {/* Identity */}
        <div className="flex items-center gap-3">
          <div className="bg-foreground/5 text-foreground-alt flex size-10 shrink-0 items-center justify-center rounded-lg">
            <LuAppWindow sx={stylexStyles.rule1} />
          </div>
          <div className="min-w-0 flex-1">
            <h2 className="text-foreground truncate text-base font-medium">
              No window in this tab
            </h2>
            <div className="text-foreground-alt text-xs">
              {earlierRun
                ? `Its window closed when ${name} last stopped.`
                : `Its window in ${name} closed.`}
            </div>
          </div>
        </div>

        {/* Windows */}
        <div className="flex flex-col gap-1">
          <div className="text-foreground-alt text-xs">Show a window here</div>
          {windows === undefined ? (
            <div className="text-foreground-alt/70 py-1.5 text-sm">
              Reading the VM's windows...
            </div>
          ) : windows.length === 0 ? (
            <div className="text-foreground-alt/70 py-1.5 text-sm">
              No windows are open. Open an app on the desktop and it appears
              here.
            </div>
          ) : (
            <ul className="flex flex-col gap-0.5">
              {windows.map((w) => (
                <li key={w.id}>
                  <button
                    type="button"
                    onClick={() => onShowWindow(w.id ?? 0)}
                    className="hover:bg-foreground/5 text-foreground flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition duration-150"
                  >
                    <LuAppWindow sx={stylexStyles.rule2} />
                    <span className="min-w-0 flex-1 truncate">
                      {windowLabel(w)}
                    </span>
                    {w.place === WasiVmWindowPlace.WasiVmWindowPlace_VIEWER && (
                      <span className="text-foreground-alt/60 text-xs">
                        In another tab
                      </span>
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
