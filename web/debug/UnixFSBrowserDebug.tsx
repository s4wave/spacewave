import { useCallback, useState, type ReactNode } from 'react'
import {
  LuArrowLeft,
  LuChevronLeft,
  LuChevronRight,
  LuChevronUp,
  LuCircleAlert,
  LuFile,
  LuFolder,
  LuFolderOpen,
  LuFolderPlus,
  LuRotateCw,
  LuUpload,
} from 'react-icons/lu'

import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'

// UnixFSBrowserDebug renders prototype variants for the UnixFS browser
// surfaces (file row, toolbar, empty state, drag overlay, error state,
// destructive dialog). Each section shows the current production look first,
// then candidate variants aligned with the app design system.
export function UnixFSBrowserDebug() {
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  return (
    <div className="bg-background flex h-full w-full flex-col overflow-auto">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center gap-2 border-b px-4">
        <button
          type="button"
          onClick={goBack}
          className="text-foreground-alt hover:text-foreground transition-colors"
          aria-label="Back"
        >
          <LuArrowLeft className="size-4" />
        </button>
        <span className="text-foreground text-sm font-semibold tracking-tight select-none">
          UnixFS Browser UI Prototype
        </span>
      </div>

      <div className="flex-1 overflow-auto px-4 py-3">
        <div className="mx-auto max-w-3xl space-y-6">
          <Intro />
          <FileRowSection />
          <ToolbarSection />
          <EmptyStateSection />
          <DragOverlaySection />
          <ErrorStateSection />
          <DestructiveDialogSection />
        </div>
      </div>
    </div>
  )
}

function Intro() {
  return (
    <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5 backdrop-blur-sm">
      <p className="text-foreground text-sm font-semibold tracking-tight">
        UnixFS browser variant gallery
      </p>
      <p className="text-foreground-alt/70 mt-1 text-xs leading-relaxed">
        Side-by-side variants for the surfaces that compose UnixFSBrowser (file
        row, toolbar, empty state, drag overlay, error state, delete dialog).
        Each section shows the current production rendering, then candidate
        variants drawn from the modern token set.
      </p>
      <p className="text-foreground-alt/70 mt-2 text-xs leading-relaxed">
        Decisions live in production code:
      </p>
      <ul className="text-foreground-alt/70 mt-1 list-disc space-y-0.5 pl-5 text-xs leading-relaxed">
        <li>
          File row: B (compact dense, modern tokens, brand left bar on selected)
          in web/editors/file-browser/FileListEntry.tsx.
        </li>
        <li>
          Toolbar: B (modern, NavIconButton-style buttons) in
          web/editors/file-browser/Toolbar.tsx.
        </li>
        <li>Empty state: A (current centered three-line) kept as-is.</li>
        <li>Drag overlay: A (current dashed border) kept as-is.</li>
        <li>
          Error state: C (inline strip under toolbar) in
          app/unixfs/UnixFSBrowser.tsx.
        </li>
        <li>
          Delete dialog buttons: B (tinted destructive outline) in
          app/unixfs/UnixFSBrowser.tsx.
        </li>
      </ul>
    </div>
  )
}

// -----------------------------------------------------------------------
// Section / Variant wrappers
// -----------------------------------------------------------------------

interface SectionProps {
  title: string
  description: string
  children: ReactNode
}

function Section({ title, description, children }: SectionProps) {
  return (
    <section className="space-y-3">
      <div>
        <h2 className="text-foreground text-sm font-semibold tracking-tight select-none">
          {title}
        </h2>
        <p className="text-foreground-alt/60 mt-0.5 text-xs">{description}</p>
      </div>
      {children}
    </section>
  )
}

interface VariantProps {
  label: string
  note?: string
  children: ReactNode
}

function Variant({ label, note, children }: VariantProps) {
  return (
    <div className="border-foreground/6 bg-background-card/30 space-y-2 rounded-lg border p-3.5 backdrop-blur-sm">
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-foreground text-xs font-medium tracking-wide select-none">
          {label}
        </span>
        {note && (
          <span className="text-foreground-alt/50 text-[0.6rem]">{note}</span>
        )}
      </div>
      <div className="border-foreground/6 bg-background/40 overflow-hidden rounded-md border">
        {children}
      </div>
    </div>
  )
}

// -----------------------------------------------------------------------
// File row
// -----------------------------------------------------------------------

interface FileRowSample {
  id: string
  name: string
  isDir: boolean
  modTime: string
  size: string
}

const FILE_ROWS: FileRowSample[] = [
  {
    id: 'a',
    name: 'documents',
    isDir: true,
    modTime: 'Apr 28, 2026',
    size: '12 items',
  },
  {
    id: 'b',
    name: 'photos',
    isDir: true,
    modTime: 'Apr 22, 2026',
    size: '3 items',
  },
  {
    id: 'c',
    name: 'spec.md',
    isDir: false,
    modTime: 'Apr 30, 2026',
    size: '4.2 KB',
  },
  {
    id: 'd',
    name: 'screenshot.png',
    isDir: false,
    modTime: 'Apr 18, 2026',
    size: '128 KB',
  },
  {
    id: 'e',
    name: 'archive.tar.gz',
    isDir: false,
    modTime: 'Mar 09, 2026',
    size: '2.4 MB',
  },
]

function FileRowSection() {
  const [selectedA, setSelectedA] = useState<string | null>('c')
  const [selectedB, setSelectedB] = useState<string | null>('c')
  const [selectedC, setSelectedC] = useState<string | null>('c')

  return (
    <Section
      title="File row (FileListEntry)"
      description="Each row in the directory list. Current rows use deprecated tokens, alternating row backgrounds, and a focus ring; design-system rows use the modern token set with hover/selection on /5 and /10 opacities."
    >
      <Variant
        label="A. Current (alternating rows + legacy tokens)"
        note="text-file-browser-row, bg-ui-selected, bg-file-row-alternate"
      >
        <div className="bg-file-back">
          {FILE_ROWS.map((row, i) => (
            <CurrentRow
              key={row.id}
              row={row}
              index={i}
              selected={selectedA === row.id}
              onSelect={() => setSelectedA(row.id)}
            />
          ))}
        </div>
      </Variant>

      <Variant
        label="B. Compact dense (modern tokens, no alternating)"
        note="hover bg-foreground/5, selection bg-brand/10 + brand left bar"
      >
        <div>
          {FILE_ROWS.map((row) => (
            <DenseRow
              key={row.id}
              row={row}
              selected={selectedB === row.id}
              onSelect={() => setSelectedB(row.id)}
            />
          ))}
        </div>
      </Variant>

      <Variant
        label="C. Card rows (rounded, gapped)"
        note="Each row a glass card; readable in lists < 12 entries"
      >
        <div className="space-y-1.5 p-2">
          {FILE_ROWS.map((row) => (
            <CardRow
              key={row.id}
              row={row}
              selected={selectedC === row.id}
              onSelect={() => setSelectedC(row.id)}
            />
          ))}
        </div>
      </Variant>
    </Section>
  )
}

function CurrentRow({
  row,
  index,
  selected,
  onSelect,
}: {
  row: FileRowSample
  index: number
  selected: boolean
  onSelect: () => void
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={(event) => {
        if (event.key !== 'Enter' && event.key !== ' ') return
        event.preventDefault()
        onSelect()
      }}
      className={cn(
        'group text-file-browser-row flex items-center px-3 py-1.5 text-xs',
        'hover:bg-outliner-selected-highlight cursor-pointer transition-colors select-none',
        selected && 'bg-ui-selected hover:bg-ui-selected',
        index % 2 === 1 && !selected && 'bg-file-row-alternate',
      )}
    >
      <div className="flex min-w-[120px] flex-1 items-center gap-2 overflow-hidden">
        {row.isDir ?
          <LuFolder className="text-file-folder-icon size-4 shrink-0" />
        : <LuFile className="text-foreground-alt size-4 shrink-0" />}
        <span className="truncate">{row.name}</span>
      </div>
      <div className="text-foreground-alt w-[140px] min-w-[100px] shrink text-xs opacity-70">
        {row.modTime}
      </div>
      <div className="text-foreground-alt w-[70px] min-w-[50px] shrink text-right text-xs opacity-70">
        {row.size}
      </div>
    </div>
  )
}

function DenseRow({
  row,
  selected,
  onSelect,
}: {
  row: FileRowSample
  selected: boolean
  onSelect: () => void
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={(event) => {
        if (event.key !== 'Enter' && event.key !== ' ') return
        event.preventDefault()
        onSelect()
      }}
      className={cn(
        'group relative flex cursor-pointer items-center px-3 py-1.5 text-xs transition-colors select-none',
        selected ?
          'bg-brand/10 text-foreground'
        : 'text-foreground/90 hover:bg-foreground/5',
      )}
    >
      {selected && (
        <span className="bg-brand/80 absolute top-1 bottom-1 left-0 w-[2px] rounded-r" />
      )}
      <div className="flex min-w-[120px] flex-1 items-center gap-2 overflow-hidden">
        {row.isDir ?
          <LuFolder
            className={cn(
              'size-4 shrink-0',
              selected ? 'text-brand' : 'text-foreground-alt/80',
            )}
          />
        : <LuFile
            className={cn(
              'size-4 shrink-0',
              selected ? 'text-foreground' : 'text-foreground-alt/60',
            )}
          />
        }
        <span className="truncate">{row.name}</span>
      </div>
      <div className="text-foreground-alt/50 w-[140px] min-w-[100px] shrink text-xs">
        {row.modTime}
      </div>
      <div className="text-foreground-alt/50 w-[70px] min-w-[50px] shrink text-right text-xs">
        {row.size}
      </div>
    </div>
  )
}

function CardRow({
  row,
  selected,
  onSelect,
}: {
  row: FileRowSample
  selected: boolean
  onSelect: () => void
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={(event) => {
        if (event.key !== 'Enter' && event.key !== ' ') return
        event.preventDefault()
        onSelect()
      }}
      className={cn(
        'group flex cursor-pointer items-center gap-3 rounded-md border px-3 py-2 text-xs transition-all duration-150 select-none',
        selected ?
          'border-brand/30 bg-brand/5'
        : 'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50',
      )}
    >
      <span
        className={cn(
          'flex size-7 shrink-0 items-center justify-center rounded-md transition-colors',
          selected ? 'bg-brand/15' : (
            'bg-foreground/5 group-hover:bg-foreground/8'
          ),
        )}
      >
        {row.isDir ?
          <LuFolder
            className={cn(
              'size-3.5',
              selected ? 'text-brand' : 'text-foreground-alt/70',
            )}
          />
        : <LuFile
            className={cn(
              'size-3.5',
              selected ? 'text-foreground' : 'text-foreground-alt/60',
            )}
          />
        }
      </span>
      <div className="min-w-0 flex-1 truncate">
        <div className="text-foreground truncate font-medium">{row.name}</div>
      </div>
      <div className="text-foreground-alt/50 hidden w-[120px] shrink truncate text-[0.65rem] sm:block">
        {row.modTime}
      </div>
      <div className="text-foreground-alt/50 w-[60px] shrink text-right text-[0.65rem]">
        {row.size}
      </div>
    </div>
  )
}

// -----------------------------------------------------------------------
// Toolbar
// -----------------------------------------------------------------------

function ToolbarSection() {
  return (
    <Section
      title="Toolbar (Toolbar.tsx + PanelHeader)"
      description="Top-of-browser nav. Current bar uses bg-panel-header and bg-pulldown-hover button states (legacy). Design-system bars use border-foreground/8 dividers and DashboardButton-style action chips."
    >
      <Variant
        label="A. Current (PanelHeader + bg-pulldown-hover)"
        note="Native button hover, no tooltips on the path bar"
      >
        <CurrentToolbar />
      </Variant>

      <Variant
        label="B. Modern toolbar"
        note="border-foreground/8 row, foreground-alt -> foreground hover, action chips"
      >
        <ModernToolbar />
      </Variant>
    </Section>
  )
}

function CurrentToolbar() {
  return (
    <div className="bg-panel-header border-foreground/8 flex h-9 items-center gap-1 border-b px-2">
      <div className="flex">
        <button
          type="button"
          aria-label="Back"
          className="hover:bg-pulldown-hover rounded p-[2px]"
        >
          <LuChevronLeft className="text-foreground-alt size-4" />
        </button>
        <button
          type="button"
          aria-label="Forward"
          className="cursor-default rounded p-[2px] opacity-40"
        >
          <LuChevronRight className="text-foreground-alt size-4" />
        </button>
        <button
          type="button"
          aria-label="Up"
          className="hover:bg-pulldown-hover rounded p-[2px]"
        >
          <LuChevronUp className="text-foreground-alt size-4" />
        </button>
      </div>
      <div className="border-foreground/10 bg-background/50 mx-1 flex flex-1 items-center gap-1 rounded border px-2 py-0.5 text-xs">
        <LuFolderOpen className="text-foreground-alt/50 size-3.5 shrink-0" />
        <span className="text-foreground/90 truncate">/photos/2026/april</span>
      </div>
      <div className="flex gap-1">
        <button
          type="button"
          title="New folder"
          className="hover:bg-pulldown-hover rounded p-[2px]"
        >
          <LuFolderPlus className="text-foreground-alt size-4" />
        </button>
        <button
          type="button"
          title="Upload files"
          className="hover:bg-pulldown-hover rounded p-[2px]"
        >
          <LuUpload className="text-foreground-alt size-4" />
        </button>
      </div>
    </div>
  )
}

function ModernToolbar() {
  return (
    <div className="border-foreground/8 flex h-9 items-center gap-1.5 border-b px-3">
      <div className="flex items-center gap-0.5">
        <NavIconButton
          icon={<LuChevronLeft className="size-3.5" />}
          label="Back"
        />
        <NavIconButton
          icon={<LuChevronRight className="size-3.5" />}
          label="Forward"
          disabled
        />
        <NavIconButton icon={<LuChevronUp className="size-3.5" />} label="Up" />
      </div>
      <div className="border-foreground/8 hover:border-foreground/15 mx-1 flex h-6 flex-1 items-center gap-1.5 rounded-md border bg-transparent px-2 transition-colors">
        <LuFolderOpen className="text-foreground-alt/60 size-3 shrink-0" />
        <span className="text-foreground-alt/80 truncate text-[11px]">
          /photos/2026/april
        </span>
      </div>
      <div className="flex items-center gap-0.5">
        <NavIconButton
          icon={<LuFolderPlus className="size-3.5" />}
          label="New folder"
        />
        <NavIconButton
          icon={<LuUpload className="size-3.5" />}
          label="Upload"
        />
      </div>
    </div>
  )
}

function NavIconButton({
  icon,
  label,
  disabled,
}: {
  icon: ReactNode
  label: string
  disabled?: boolean
}) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      disabled={disabled}
      className={cn(
        'flex size-6 items-center justify-center rounded transition-colors',
        disabled ?
          'text-foreground-alt/30 cursor-default'
        : 'text-foreground-alt hover:text-foreground hover:bg-foreground/5',
      )}
    >
      {icon}
    </button>
  )
}

// -----------------------------------------------------------------------
// Empty states
// -----------------------------------------------------------------------

function EmptyStateSection() {
  return (
    <Section
      title="Empty state (no UnixFS object)"
      description="Shown when rootHandle has no value. Current state stacks three muted lines centered on bg-file-back. Design-system pattern is a compact card with muted icon + headline."
    >
      <Variant
        label="A. Current (centered three-line)"
        note="text-foreground-alt + text-foreground-alt/70, no icon"
      >
        <div className="bg-file-back flex min-h-[160px] items-center justify-center">
          <div className="flex flex-col items-center text-center">
            <div className="text-foreground-alt text-sm">
              UnixFS object not found
            </div>
            <div className="text-foreground-alt/70 mt-1 text-xs">
              Object: 0x0a1b2c3d4e5f…
            </div>
            <div className="text-foreground-alt/70 mt-2 text-xs">
              Create a drive via quickstart to initialize demo content.
            </div>
          </div>
        </div>
      </Variant>

      <Variant
        label="B. Compact card (design-system)"
        note="border-foreground/6 bg-background-card/30, muted icon + headline + helper"
      >
        <div className="bg-file-back flex min-h-[160px] items-center justify-center p-6">
          <div className="border-foreground/6 bg-background-card/30 w-full max-w-xs rounded-lg border p-4 backdrop-blur-sm">
            <div className="flex items-start gap-2.5">
              <span className="bg-foreground/5 flex size-8 shrink-0 items-center justify-center rounded-md">
                <LuFolder className="text-foreground-alt/60 size-4" />
              </span>
              <div className="min-w-0">
                <p className="text-foreground text-xs font-medium select-none">
                  UnixFS object not found
                </p>
                <p className="text-foreground-alt/60 mt-0.5 text-[11px] leading-relaxed">
                  Create a drive via quickstart to initialize demo content.
                </p>
                <p className="text-foreground-alt/40 mt-1 font-mono text-[10px]">
                  0x0a1b2c3d4e5f…
                </p>
              </div>
            </div>
          </div>
        </div>
      </Variant>

      <Variant
        label="C. Single-line muted"
        note="Matches design-system 'compact empty state', text-foreground-alt/40"
      >
        <div className="bg-file-back flex min-h-[160px] items-center justify-center p-6">
          <div className="text-foreground-alt/40 flex items-center gap-2 text-xs">
            <LuFolder className="size-3.5 shrink-0" />
            <span>No UnixFS object yet. Create a drive via quickstart.</span>
          </div>
        </div>
      </Variant>
    </Section>
  )
}

// -----------------------------------------------------------------------
// Drag overlay
// -----------------------------------------------------------------------

function DragOverlaySection() {
  return (
    <Section
      title="Drag overlay (drop-to-upload)"
      description="Shown over the file list while files are being dragged in. Current overlay uses border-2 border-dashed and a large icon. Design-system tightens the chrome and uses the glass stack."
    >
      <Variant
        label="A. Current (large dashed border + h-8 icon)"
        note="border-brand/50, bg-brand/5, text-sm"
      >
        <div className="bg-file-back relative h-[180px]">
          <DummyRows />
          <div className="border-brand/50 bg-brand/5 pointer-events-none absolute inset-0 z-10 flex items-center justify-center rounded-md border-2 border-dashed">
            <div className="flex flex-col items-center gap-2">
              <LuUpload className="text-brand size-8" />
              <span className="text-brand text-sm font-medium">
                Drop files to upload
              </span>
            </div>
          </div>
        </div>
      </Variant>

      <Variant
        label="B. Glass card overlay"
        note="border-brand/30 dashed, glass background, compact icon + caption"
      >
        <div className="bg-file-back relative h-[180px]">
          <DummyRows />
          <div className="border-brand/30 bg-brand/5 pointer-events-none absolute inset-0 z-10 flex items-center justify-center rounded-md border border-dashed backdrop-blur-sm">
            <div className="border-brand/30 bg-background-card/40 flex items-center gap-2.5 rounded-lg border px-3.5 py-2 backdrop-blur-sm">
              <span className="bg-brand/15 flex size-7 shrink-0 items-center justify-center rounded-md">
                <LuUpload className="text-brand size-3.5" />
              </span>
              <div className="min-w-0">
                <p className="text-foreground text-xs font-medium select-none">
                  Drop files to upload
                </p>
                <p className="text-foreground-alt/60 text-[11px]">
                  to /photos/2026/april
                </p>
              </div>
            </div>
          </div>
        </div>
      </Variant>
    </Section>
  )
}

function DummyRows() {
  return (
    <div className="opacity-60">
      {FILE_ROWS.slice(0, 4).map((row) => (
        <div key={row.id} className="flex items-center px-3 py-1.5 text-xs">
          <div className="flex min-w-[120px] flex-1 items-center gap-2 overflow-hidden">
            {row.isDir ?
              <LuFolder className="text-foreground-alt/60 size-4 shrink-0" />
            : <LuFile className="text-foreground-alt/60 size-4 shrink-0" />}
            <span className="text-foreground-alt/70 truncate">{row.name}</span>
          </div>
        </div>
      ))}
    </div>
  )
}

// -----------------------------------------------------------------------
// Error state
// -----------------------------------------------------------------------

function ErrorStateSection() {
  return (
    <Section
      title="Error state (load failure)"
      description="Shown when any of root/path/stat/entries resources error. Current state is plain text + an underline retry link; design-system patterns use a tinted destructive callout with a button."
    >
      <Variant
        label="A. Current (plain text + underline link)"
        note="text-destructive headline, text-brand underline retry"
      >
        <div className="bg-file-back flex min-h-[160px] flex-col items-center justify-center p-6">
          <div className="text-destructive text-xs">Error loading files</div>
          <div className="text-foreground-alt/70 mt-1 text-xs">
            failed to read directory: permission denied
          </div>
          <button className="text-brand mt-2 text-xs underline">Retry</button>
        </div>
      </Variant>

      <Variant
        label="B. Tinted destructive callout"
        note="border-destructive/20 bg-destructive/5 + LuCircleAlert + retry chip"
      >
        <div className="bg-file-back flex min-h-[160px] items-center justify-center p-6">
          <div className="border-destructive/20 bg-destructive/5 w-full max-w-sm rounded-lg border p-3.5">
            <div className="flex items-start gap-2.5">
              <LuCircleAlert className="text-destructive mt-0.5 size-3.5 shrink-0" />
              <div className="min-w-0 flex-1">
                <p className="text-foreground text-xs font-medium select-none">
                  Error loading files
                </p>
                <p className="text-foreground-alt/70 mt-0.5 text-[11px] leading-relaxed">
                  failed to read directory: permission denied
                </p>
              </div>
            </div>
            <div className="mt-3 flex justify-end">
              <button
                type="button"
                className="border-destructive/30 bg-destructive/10 hover:border-destructive/50 hover:bg-destructive/15 text-foreground inline-flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium transition-all duration-150"
              >
                <LuRotateCw className="size-3" />
                Retry
              </button>
            </div>
          </div>
        </div>
      </Variant>

      <Variant
        label="C. Inline strip (one-line)"
        note="Slim banner, fits under the toolbar without disrupting the file list"
      >
        <div className="bg-file-back min-h-[160px]">
          <div className="border-destructive/20 bg-destructive/5 flex items-center gap-2 border-b px-3 py-1.5">
            <LuCircleAlert className="text-destructive size-3.5 shrink-0" />
            <p className="text-foreground/80 min-w-0 flex-1 truncate text-xs">
              Error loading files: permission denied
            </p>
            <button
              type="button"
              className="text-foreground-alt hover:text-foreground inline-flex items-center gap-1 text-xs font-medium transition-colors"
            >
              <LuRotateCw className="size-3" />
              Retry
            </button>
          </div>
          <DummyRows />
        </div>
      </Variant>
    </Section>
  )
}

// -----------------------------------------------------------------------
// Destructive (delete) dialog buttons
// -----------------------------------------------------------------------

function DestructiveDialogSection() {
  return (
    <Section
      title="Delete confirmation buttons"
      description="DialogFooter buttons for the delete dialog. Current uses a solid destructive fill ('bg-destructive text-destructive-foreground'); design-system primary actions use tinted outlines with /30 borders and /10 fills."
    >
      <Variant
        label="A. Current (solid destructive)"
        note="bg-destructive + text-destructive-foreground"
      >
        <div className="bg-background flex justify-end gap-2 p-3">
          <button className="hover:bg-accent rounded-md px-4 py-2 text-xs">
            Cancel
          </button>
          <button className="bg-destructive text-destructive-foreground hover:bg-destructive/90 rounded-md px-4 py-2 text-xs">
            Delete
          </button>
        </div>
      </Variant>

      <Variant
        label="B. Tinted destructive outline"
        note="border-destructive/30 bg-destructive/10 hover:bg-destructive/15"
      >
        <div className="bg-background flex justify-end gap-2 p-3">
          <button className="text-foreground-alt hover:text-foreground h-7 rounded-md px-3 text-xs transition-colors">
            Cancel
          </button>
          <button className="border-destructive/30 bg-destructive/10 hover:border-destructive/50 hover:bg-destructive/15 text-foreground h-7 rounded-md border px-3 text-xs font-medium transition-all duration-150">
            Delete
          </button>
        </div>
      </Variant>
    </Section>
  )
}
