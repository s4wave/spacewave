import { useState } from 'react'
import {
  LuBookOpen,
  LuChartLine,
  LuFile,
  LuMenu,
  LuPenLine,
  LuPlus,
  LuSave,
  LuX,
} from 'react-icons/lu'

import { Toolbar } from '@s4wave/web/editors/file-browser/Toolbar.js'
import {
  ViewerBar,
  type ViewerBarAction,
} from '@s4wave/web/object/ViewerBar.js'
import { useParams } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'

const noop = () => {}

// ranges are a Dashboard's time range presets.
const ranges = ['15m', '1h', '6h', '24h', '7d'].map((id) => ({
  id,
  label: id,
}))

// PromBarDebug renders a Dashboard's bar with its time range as the bar's
// choice, for the viewer sweep. State "readonly" leaves out Add panel.
export function PromBarDebug() {
  const { state } = useParams()
  const [range, setRange] = useState('1h')
  const actions: ViewerBarAction[] =
    state === 'readonly'
      ? []
      : [{ id: 'add', label: 'Add panel', icon: <LuPlus />, onClick: noop }]
  return (
    <div className="bg-background viewer-pane flex h-full w-full flex-col">
      <h1 className="sr-only">Dashboard bar</h1>
      <ViewerBar
        icon={<LuChartLine />}
        title="Dashboard"
        description="Updated 19:30"
        choice={{
          label: 'Time range',
          options: ranges,
          value: range,
          onChange: setRange,
        }}
        actions={actions}
      />
      <div className="text-foreground-alt min-h-0 flex-1 p-3 text-xs">
        Panels over the last {range}.
      </div>
    </div>
  )
}

// pages are the page list the documentation fixture shows.
const pages = ['Getting started', 'Spaces', 'Flowgraphs', 'Devices']

// DocsBarDebug renders a documentation viewer's bar over its page list and
// page, for the viewer sweep. The page list is a side column from a 42rem
// pane; below it the bar's narrow Pages press opens it as a drawer. State
// "editing" swaps Edit for Save and Cancel.
export function DocsBarDebug() {
  // Pages leads both modes; editing swaps Edit for Cancel and Save.
  const { state } = useParams()
  const editing = state === 'editing'
  const pagesPress: ViewerBarAction = {
    id: 'pages',
    label: 'Pages',
    icon: <LuMenu />,
    iconOnly: true,
    narrow: true,
    onClick: noop,
  }
  const actions: ViewerBarAction[] = editing
    ? [
        pagesPress,
        { id: 'cancel', label: 'Cancel', icon: <LuX />, onClick: noop },
        {
          id: 'save',
          label: 'Save',
          icon: <LuSave />,
          variant: 'primary',
          onClick: noop,
        },
      ]
    : [
        pagesPress,
        { id: 'edit', label: 'Edit page', icon: <LuPenLine />, onClick: noop },
      ]
  return (
    <div className="bg-background viewer-pane flex h-full w-full flex-col">
      <h1 className="sr-only">Documentation bar</h1>
      <ViewerBar icon={<LuBookOpen />} title="Spaces" actions={actions} />
      <div className="flex min-h-0 flex-1">
        <nav
          aria-label="Pages"
          className="border-border @pane-2xl:flex hidden w-55 shrink-0 flex-col border-r py-1"
        >
          {pages.map((page) => (
            <button
              key={page}
              type="button"
              aria-current={page === 'Spaces' ? 'page' : undefined}
              className={cn(
                'h-viewer-row flex items-center gap-2 px-3 text-left text-xs',
                page === 'Spaces'
                  ? 'bg-foreground/8 text-foreground'
                  : 'text-foreground-alt hover:bg-foreground/5',
              )}
            >
              <LuFile className="size-3 shrink-0" aria-hidden />
              <span className="truncate">{page}</span>
            </button>
          ))}
        </nav>
        <div className="text-foreground-alt min-w-0 flex-1 p-4 text-xs">
          A Space holds the objects a person shares with their Devices.
        </div>
      </div>
    </div>
  )
}

// DriveBarDebug renders the Drive's own file-browser toolbar, unchanged, at
// the viewer bar's 28px height, for the viewer sweep. State "current" draws
// it at its own 25px height to compare.
export function DriveBarDebug() {
  const { state } = useParams()
  return (
    <div className="bg-background viewer-pane flex h-full w-full flex-col">
      <h1 className="sr-only">Drive toolbar</h1>
      <Toolbar
        currentPath="/photos/2026/trip"
        canGoBack
        canGoUp
        upDropPath="/photos/2026"
        onBack={noop}
        onForward={noop}
        onUp={noop}
        onNavigate={noop}
        onNewFolder={noop}
        onUploadFiles={noop}
        height={state === 'current' ? 25 : 28}
      />
      <div className="text-foreground-alt min-h-0 flex-1 p-3 text-xs">
        The file list keeps its rows, selection and keyboard navigation.
      </div>
    </div>
  )
}
