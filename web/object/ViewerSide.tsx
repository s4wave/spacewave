import { useEffect, useRef, useState, type ReactNode } from 'react'
import { LuChevronLeft, LuChevronRight } from 'react-icons/lu'

import { FactList } from '@s4wave/web/ui/FactList.js'

// ViewerSideState is the column's state from a 42rem pane: open with its
// facts, or a rail of state glyphs.
export type ViewerSideState = 'open' | 'rail'

// ViewerSideProps configures a ViewerSide.
export interface ViewerSideProps {
  // label names the column, its Collapse and Expand presses, and the
  // region for assistive technology, such as "Device".
  label: string
  // facts are the FactRows the column lists; the bar's Details popover
  // lists the same facts below 42rem.
  facts: ReactNode
  // rail is the state glyphs the collapsed column keeps, each with its own
  // accessible name, such as a failed update's alert.
  rail?: ReactNode
  // initial is the column's first state. Defaults to open.
  initial?: ViewerSideState
  // onChange hears each Collapse and Expand, so a viewer that unmounts the
  // column, such as on a view switch, can pass the state back as initial.
  onChange?: (side: ViewerSideState) => void
  // children is the viewer's work beside the column; it scrolls on its own.
  children?: ReactNode
}

// pressClass is a 24px icon press, the size of a viewer row.
const pressClass =
  'focus-ring text-foreground-alt hover:bg-foreground/5 hover:text-foreground flex size-6 shrink-0 items-center justify-center rounded transition-colors [&_svg]:size-3.5 pointer-coarse:size-11'

/**
 * ViewerSide lays out a viewer's body with a side column of facts at the
 * pane's left edge. From a 42rem pane the column is viewer-side wide, or a
 * viewer-rail of state glyphs when collapsed; below 42rem it is gone and the
 * bar's Details press opens the same facts. The column's state outlives a
 * narrow spell. ViewerSide.tla checks that the facts show or are one press
 * away at every width.
 *
 * Collapse and Expand replace each other in place, and focus follows the
 * press, so a keyboard user can toggle the column without losing their place.
 * Each carries data-viewer-side-press, and the column's root data-viewer-side,
 * where ViewerBar sends focus when a widening pane closes its Details popover;
 * ViewerBar looks only in a column that is its own sibling.
 */
export function ViewerSide({
  label,
  facts,
  rail,
  initial = 'open',
  onChange,
  children,
}: ViewerSideProps) {
  // Hold the column's state and the press that toggles it.
  const [side, setSide] = useState<ViewerSideState>(initial)
  const pressRef = useRef<HTMLButtonElement>(null)
  const pressedRef = useRef(false)
  const name = label.toLowerCase()

  // Move focus to the press that replaced the one just pressed.
  useEffect(() => {
    if (!pressedRef.current) return
    pressedRef.current = false
    pressRef.current?.focus()
  }, [side])

  // toggle switches the column and remembers that a press did it.
  const toggle = (next: ViewerSideState) => {
    pressedRef.current = true
    setSide(next)
    onChange?.(next)
  }

  return (
    <div data-viewer-side className="flex min-h-0 flex-1">
      {side === 'open' ? (
        <aside
          aria-label={label}
          className="border-foreground/8 w-viewer-side @pane-2xl:flex hidden shrink-0 flex-col overflow-y-auto border-e"
        >
          <div className="h-viewer-row text-foreground-alt bg-background-primary sticky top-0 z-10 flex shrink-0 items-center justify-between ps-2 pe-1 text-xs font-medium select-none">
            <span className="min-w-0 truncate">{label}</span>
            <button
              ref={pressRef}
              type="button"
              data-viewer-side-press
              aria-label={`Collapse ${name}`}
              title={`Collapse ${name}`}
              className={pressClass}
              onClick={() => toggle('rail')}
            >
              <LuChevronLeft aria-hidden />
            </button>
          </div>
          <FactList dense className="ms-2 me-1">
            {facts}
          </FactList>
        </aside>
      ) : (
        <aside
          aria-label={label}
          className="border-foreground/8 w-viewer-rail @pane-2xl:flex hidden shrink-0 flex-col items-center gap-1 border-e py-1"
        >
          <button
            ref={pressRef}
            type="button"
            data-viewer-side-press
            aria-label={`Expand ${name}`}
            title={`Expand ${name}`}
            className={pressClass}
            onClick={() => toggle('open')}
          >
            <LuChevronRight aria-hidden />
          </button>
          {rail}
        </aside>
      )}
      <div className="flex min-w-0 flex-1 flex-col overflow-y-auto">
        {children}
      </div>
    </div>
  )
}

// ViewerRailGlyphProps configures a ViewerRailGlyph.
export interface ViewerRailGlyphProps {
  // label names the state the glyph shows, such as "Update failed".
  label: string
  // icon is the glyph; it keeps its own tone.
  icon: ReactNode
}

// ViewerRailGlyph is one state glyph on a collapsed side column's rail, a
// viewer row square, named for assistive technology.
export function ViewerRailGlyph({ label, icon }: ViewerRailGlyphProps) {
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      className="flex size-6 shrink-0 items-center justify-center [&_svg]:size-3.5"
    >
      {icon}
    </span>
  )
}
