import {
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from 'react'
import { LuEllipsis, LuInfo } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'
import { FactList } from '@s4wave/web/ui/FactList.js'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@s4wave/web/ui/Popover.js'

/**
 * ViewerBarFold is where a press leaves the bar for More: "xs" below a 20rem
 * pane, "md" below 28rem, and "menu" at every width.
 */
export type ViewerBarFold = 'xs' | 'md' | 'menu'

/** ViewerBarActionBase is what every press on the object a viewer shows has. */
interface ViewerBarActionBase {
  // id keys the press; it stays fixed while the label changes.
  id: string
  // label names the press, its menu item, and its tooltip while the bar draws
  // it as an icon.
  label: string
  icon?: ReactNode
  onClick: () => void
  // disabled marks a press whose action is running, and its label says so. The
  // press stays focusable and ignores presses, so focus stays on it. A press
  // that cannot work in the object's state is left out, not disabled.
  disabled?: boolean
  variant?: 'default' | 'primary' | 'destructive'
  // iconOnly draws the press as its icon alone at every width. Otherwise a
  // press with an icon shows its label from a 28rem pane.
  iconOnly?: boolean
}

/**
 * ViewerBarAction is one press on the object a viewer shows. A narrow press
 * shows only below a 42rem pane, where the side column it opens is gone, such
 * as a Pages press beside a page list; from 42rem it is gone, not folded, so
 * More never lists it. A narrow press folds at "xs" or "md", never "menu",
 * since More would then show at 42rem with nothing to list.
 */
export type ViewerBarAction = ViewerBarActionBase &
  (
    | {
        narrow?: false
        // fold defaults to "xs".
        fold?: ViewerBarFold
      }
    | { narrow: true; fold?: 'xs' | 'md' }
  )

/** ViewerBarOption is one option of a choice. */
export interface ViewerBarOption {
  id: string
  label: string
}

/**
 * ViewerBarChoice is a group of exclusive options in the bar, such as a time
 * range. From a 28rem pane the options sit in the bar, the current one
 * pressed; below it one press names the current option and opens a menu of
 * them. The choice never folds into More. ViewerBarChoice.tla holds its
 * states.
 */
export interface ViewerBarChoice {
  // label names the group, its press and its menu, such as "Time range".
  label: string
  options: ViewerBarOption[]
  // value is the current option's id.
  value: string
  onChange: (id: string) => void
}

interface ViewerBarProps {
  // icon marks the object's kind; it keeps its own color.
  icon?: ReactNode
  // title names the object; it keeps viewer-title of width at every size.
  title: string
  // tone "alert" draws the title in the destructive color.
  tone?: 'alert'
  // state is a short chip such as a badge, kept whole.
  state?: ReactNode
  // description is one muted line of what the object is, from a 28rem pane;
  // below that More shows it above the folded presses.
  description?: ReactNode
  actions?: ViewerBarAction[]
  // choice puts a group of exclusive options before the presses.
  choice?: ViewerBarChoice
  // details puts a Details press before More that opens the object's facts.
  details?: ViewerBarDetails
  className?: string
}

/**
 * ViewerBarDetails is the bar's Details press and the facts its popover
 * lists. The press never folds into More, since a menu cannot hold the
 * popover's copy presses.
 */
export interface ViewerBarDetails {
  // label heads the popover and names it, such as "Device".
  label: string
  // facts are the FactRows the popover lists.
  facts: ReactNode
  // narrow keeps the press to panes below 42rem, where the side column that
  // holds the same facts is gone. A popover open when the pane widens past
  // 42rem closes.
  narrow?: boolean
  // defaultOpen opens the popover on mount, for a fixture that shows it.
  defaultOpen?: boolean
}

const foldClass: Record<ViewerBarFold, string> = {
  xs: 'hidden @pane-xs:inline-flex',
  md: 'hidden @pane-md:inline-flex',
  menu: 'hidden',
}

const pressTone: Record<NonNullable<ViewerBarAction['variant']>, string> = {
  default: 'text-foreground-alt hover:bg-foreground/5 hover:text-foreground',
  primary:
    'border border-brand/30 bg-brand/10 text-foreground hover:border-brand/50 hover:bg-brand/15',
  destructive: 'text-destructive hover:bg-destructive/10',
}

const pressClass =
  'focus-ring h-viewer-press shrink-0 items-center justify-center gap-1 rounded px-2 text-xs whitespace-nowrap select-none transition-colors aria-disabled:pointer-events-none aria-disabled:opacity-50 [&_svg]:size-3.5 pointer-coarse:min-h-11'

const iconPressClass = 'w-viewer-press px-0 pointer-coarse:min-w-11'

// labelFoldClass draws a labeled press with an icon as its icon below a 28rem
// pane, so the presses that fit beside the title's floor stay in the bar.
const labelFoldClass =
  'w-viewer-press px-0 pointer-coarse:min-w-11 @pane-md:w-auto @pane-md:px-2 @pane-md:pointer-coarse:min-w-0'

// moreClass shows More at exactly the widths where a press or the description
// is folded, so the bar draws it only when it lists something.
function moreClass(
  actions: ViewerBarAction[],
  described: boolean,
): string | null {
  // Collect the fold points of every press and the folded description.
  const folds = new Set(actions.map((a) => a.fold ?? 'xs'))
  if (described) folds.add('md')
  if (folds.has('menu')) return 'inline-flex'
  if (folds.has('md')) return 'inline-flex @pane-md:hidden'
  if (folds.has('xs')) return 'inline-flex @pane-xs:hidden'
  return null
}

// MoreList is what an open More lists: the folded presses by id, and whether
// the description is folded.
interface MoreList {
  ids: string[]
  note: boolean
}

// sameList reports whether two More lists show the same presses and note.
function sameList(a: MoreList | null, b: MoreList | null): boolean {
  if (!a || !b) return a === b
  return (
    a.note === b.note &&
    a.ids.length === b.ids.length &&
    a.ids.every((id, i) => id === b.ids[i])
  )
}

function isHidden(el: Element | null | undefined): boolean {
  return !el || el.getClientRects().length === 0
}

// isGone reports whether a narrow press has left the bar at a 42rem pane, as
// opposed to folding into More.
function isGone(el: Element | null | undefined): boolean {
  const wrap = el?.parentElement
  return (
    !!wrap?.hasAttribute('data-viewer-narrow') &&
    getComputedStyle(wrap).display === 'none'
  )
}

// focusBar moves focus to the bar's first shown press, or to the bar itself
// when it shows none, so focus never falls to the page.
function focusBar(header: HTMLElement | null) {
  if (!header) return
  const presses = Array.from(header.querySelectorAll('button'))
  const first = presses.find((el) => !isHidden(el))
  ;(first ?? header).focus()
}

// focusSide moves focus to the Collapse or Expand press of the side column
// beside the bar, a sibling marked data-viewer-side, or to the bar when the
// pane shows none, so a popover closed by a widening pane never drops focus
// to the page. A column of a viewer embedded deeper in the body is not the
// bar's, so its press is skipped.
function focusSide(header: HTMLElement | null) {
  const parent = header?.parentElement
  const presses = parent?.querySelectorAll<HTMLElement>(
    '[data-viewer-side-press]',
  )
  const press = Array.from(presses ?? []).find(
    (el) =>
      el.closest('[data-viewer-side]')?.parentElement === parent &&
      !isHidden(el),
  )
  if (press) press.focus()
  else focusBar(header)
}

/**
 * ViewerBar is the one-line bar over an object viewer: the object's icon,
 * title, state and a muted line of what it is, then its presses and More. It
 * is one viewer-bar line at every pane size. As the pane narrows the muted
 * line leaves below 28rem and labeled presses become icons, then each press
 * folds into More at its fold point. The title keeps viewer-title of width.
 * A running press stays in place, disabled but focusable.
 *
 * The @pane-* variants decide which presses sit in the bar, so the bar must
 * render inside the viewer slot's pane container; outside one every press
 * folds into More. More reads what the pane hides when it opens and again on
 * each resize or change of the presses while open, so the bar and the menu
 * never disagree. It closes when nothing is folded and sends focus to the
 * first shown press, or the bar. ViewerBar.tla checks that
 * every press is in the bar or one More press away at every width.
 */
export function ViewerBar({
  icon,
  title,
  tone,
  state,
  description,
  actions = [],
  choice,
  details,
  className,
}: ViewerBarProps) {
  // Hold the bar, its description and each press, to read what the pane folds.
  const headerRef = useRef<HTMLElement>(null)
  const noteRef = useRef<HTMLParagraphElement>(null)
  const pressRefs = useRef(new Map<string, HTMLButtonElement>())
  const more = moreClass(actions, !!description)

  // readFolded lists the presses and description the pane hides now.
  const readFolded = (): MoreList => ({
    ids: actions
      .filter((a) => {
        const el = pressRefs.current.get(a.id)
        return isHidden(el) && !isGone(el)
      })
      .map((a) => a.id),
    note: !!description && isHidden(noteRef.current),
  })

  return (
    <header
      ref={headerRef}
      tabIndex={-1}
      className={cn(
        'focus-ring border-foreground/8 h-viewer-bar ps-viewer-pad @pane-roomy:ps-viewer-pad-roomy flex shrink-0 items-center gap-2 border-b pe-1 pointer-coarse:h-11',
        className,
      )}
    >
      {icon ? (
        <span
          className="text-foreground-alt flex shrink-0 [&_svg]:size-3.5"
          aria-hidden
        >
          {icon}
        </span>
      ) : null}
      <div className="min-w-viewer-title flex flex-1 items-center gap-2 overflow-hidden">
        <h2
          className={cn(
            'text-viewer-title min-w-viewer-title shrink truncate select-none',
            tone === 'alert' ? 'text-destructive' : 'text-foreground',
          )}
          title={title}
        >
          {title}
        </h2>
        {state ? (
          <span className="flex shrink-0 items-center">{state}</span>
        ) : null}
        {description ? (
          <p
            ref={noteRef}
            className="text-foreground-alt @pane-md:block hidden min-w-0 shrink-1000 truncate text-xs"
          >
            {description}
          </p>
        ) : null}
      </div>
      {actions.length > 0 || more || details || choice ? (
        <div className="flex shrink-0 items-center gap-1">
          {choice ? <ViewerChoice barRef={headerRef} {...choice} /> : null}
          {actions.map((action) => {
            const press = (
              <ViewerBarPress
                key={action.id}
                action={action}
                pressRef={(el) => {
                  if (el) pressRefs.current.set(action.id, el)
                  else pressRefs.current.delete(action.id)
                }}
              />
            )
            if (!action.narrow) return press
            return (
              <span
                key={action.id}
                data-viewer-narrow
                className="@pane-2xl:hidden contents"
              >
                {press}
              </span>
            )
          })}
          {details ? (
            <ViewerDetailsPress barRef={headerRef} {...details} />
          ) : null}
          {more ? (
            <ViewerBarMore
              className={more}
              headerRef={headerRef}
              actions={actions}
              description={description}
              readFolded={readFolded}
            />
          ) : null}
        </div>
      ) : null}
    </header>
  )
}

// ViewerBarPress is one press in the bar. A press with an icon and a label
// shows its label from a 28rem pane; the tooltip and accessible name carry the
// label whenever the bar draws only the icon.
function ViewerBarPress({
  action,
  pressRef,
}: {
  action: ViewerBarAction
  pressRef: (el: HTMLButtonElement | null) => void
}) {
  // Decide whether the label shows, folds below 28rem, or never shows.
  const hasIcon = !!action.icon
  const iconOnly = hasIcon && !!action.iconOnly
  const labelFolds = hasIcon && !action.iconOnly
  const named = iconOnly || labelFolds
  let label: ReactNode = action.label
  if (iconOnly) label = null
  else if (labelFolds) {
    label = <span className="@pane-md:inline hidden">{action.label}</span>
  }
  return (
    <button
      ref={pressRef}
      type="button"
      onClick={action.disabled ? undefined : action.onClick}
      aria-disabled={action.disabled || undefined}
      aria-label={named ? action.label : undefined}
      title={named ? action.label : undefined}
      className={cn(
        pressClass,
        pressTone[action.variant ?? 'default'],
        iconOnly && iconPressClass,
        labelFolds && labelFoldClass,
        foldClass[action.fold ?? 'xs'],
      )}
    >
      <span className="contents" aria-hidden>
        {action.icon}
      </span>
      {label}
    </button>
  )
}

// ViewerChoice is the bar's choice: its options as pressed buttons from a
// 28rem pane, and below it one press naming the current option that opens a
// menu of them. While the menu is open it watches the bar and closes once the
// press leaves it, sending focus to the current option, which the wider pane
// now shows.
function ViewerChoice({
  barRef,
  label,
  options,
  value,
  onChange,
}: ViewerBarChoice & { barRef: RefObject<HTMLElement | null> }) {
  // Hold the press, to see when it leaves the bar, and the group, to focus the
  // current option then.
  const pressRef = useRef<HTMLButtonElement>(null)
  const groupRef = useRef<HTMLDivElement>(null)
  const [open, setOpen] = useState(false)
  const onResize = useEffectEvent(() => {
    if (isHidden(pressRef.current)) setOpen(false)
  })

  // Close the menu once the pane widens its press out of the bar.
  useEffect(() => {
    // Watch the bar only while the menu is open.
    const header = barRef.current
    if (!open || !header) return
    const observer = new ResizeObserver(() => onResize())
    observer.observe(header)
    return () => observer.disconnect()
  }, [open, barRef])

  // Draw the group from 28rem and the press below it; both name the current
  // option.
  const current = options.find((o) => o.id === value)
  return (
    <>
      <div
        ref={groupRef}
        role="group"
        aria-label={label}
        className="@pane-md:flex hidden items-center gap-0.5"
      >
        {options.map((o) => (
          <button
            key={o.id}
            type="button"
            aria-pressed={o.id === value}
            onClick={() => onChange(o.id)}
            className={cn(
              pressClass,
              'inline-flex px-1.5 tabular-nums',
              o.id === value
                ? 'bg-foreground/10 text-foreground'
                : pressTone.default,
            )}
          >
            {o.label}
          </button>
        ))}
      </div>
      <DropdownMenu open={open} onOpenChange={setOpen}>
        <DropdownMenuTrigger asChild>
          <button
            ref={pressRef}
            type="button"
            aria-label={`${label}: ${current?.label ?? ''}`}
            title={label}
            className={cn(
              pressClass,
              pressTone.default,
              '@pane-md:hidden inline-flex tabular-nums',
            )}
          >
            {current?.label}
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="end"
          variant="compact"
          aria-label={label}
          onCloseAutoFocus={(e) => {
            if (!isHidden(pressRef.current)) return
            e.preventDefault()
            groupRef.current
              ?.querySelector<HTMLElement>('[aria-pressed="true"]')
              ?.focus()
          }}
        >
          {options.map((o) => (
            <DropdownMenuItem
              key={o.id}
              role="menuitemradio"
              aria-checked={o.id === value}
              variant={o.id === value ? 'selected' : 'default'}
              onSelect={() => onChange(o.id)}
            >
              {o.label}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </>
  )
}

// ViewerBarMore is the bar's More press and menu. While open it rereads what
// the pane folds on each resize of the bar and each change of the presses, and
// closes once its own press leaves the bar or nothing is folded. Closing with
// its press gone sends focus to the bar instead of the page.
function ViewerBarMore({
  className,
  headerRef,
  actions,
  description,
  readFolded,
}: {
  className: string
  headerRef: RefObject<HTMLElement | null>
  actions: ViewerBarAction[]
  description?: ReactNode
  readFolded: () => MoreList
}) {
  // Hold the More press and what its open menu lists; null is closed.
  const moreRef = useRef<HTMLButtonElement>(null)
  const [listed, setListed] = useState<MoreList | null>(null)
  const open = listed !== null

  // read lists what is folded, or null while More itself is hidden.
  const read = (): MoreList | null => {
    if (isHidden(moreRef.current)) return null
    const next = readFolded()
    return next.ids.length > 0 || next.note ? next : null
  }

  // Keep the current list when the reread matches it, so a class change that
  // folds nothing new does not render the menu again.
  const onReread = useEffectEvent(() =>
    setListed((prev) => {
      const next = read()
      return sameList(prev, next) ? prev : next
    }),
  )

  // While open, reread when the bar resizes, and when a press joins, leaves,
  // or changes its fold class. The menu renders in a portal, so its own
  // updates do not touch the bar.
  useEffect(() => {
    // Watch the bar only while the menu is open.
    const header = headerRef.current
    if (!open || !header) return
    const resize = new ResizeObserver(() => onReread())
    resize.observe(header)
    const mutation = new MutationObserver(() => onReread())
    mutation.observe(header, {
      childList: true,
      subtree: true,
      attributeFilter: ['class'],
    })
    return () => {
      resize.disconnect()
      mutation.disconnect()
    }
  }, [open, headerRef])

  // Look up the listed presses by id, in the bar's order.
  const byId = new Map(actions.map((a) => [a.id, a]))
  const items = (listed?.ids ?? []).flatMap((id) => byId.get(id) ?? [])

  return (
    <DropdownMenu
      open={open}
      onOpenChange={(next) => setListed(next ? read() : null)}
    >
      <DropdownMenuTrigger asChild>
        <button
          ref={moreRef}
          type="button"
          aria-label="More"
          title="More"
          className={cn(
            pressClass,
            pressTone.default,
            iconPressClass,
            className,
          )}
        >
          <LuEllipsis aria-hidden />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        variant="compact"
        className="max-w-72"
        onCloseAutoFocus={(e) => {
          if (!isHidden(moreRef.current)) return
          e.preventDefault()
          focusBar(headerRef.current)
        }}
      >
        {listed?.note ? (
          <DropdownMenuLabel>{description}</DropdownMenuLabel>
        ) : null}
        {listed?.note && items.length > 0 ? <DropdownMenuSeparator /> : null}
        {items.map((action) => (
          <DropdownMenuItem
            key={action.id}
            disabled={action.disabled}
            variant={
              action.variant === 'destructive' ? 'destructive' : 'default'
            }
            onSelect={action.onClick}
          >
            <span className="contents" aria-hidden>
              {action.icon}
            </span>
            {action.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/**
 * ViewerDetailsPress is the Details press and its popover of facts, for
 * ViewerBar or another viewer header such as a toolbar. The popover is the
 * Popover primitive's: opening moves focus into it, Escape or a click away
 * closes it and returns focus to the press. While open it watches the bar
 * and closes once the press leaves it, as a narrow press does from a 42rem
 * pane, where the side column shows the facts; focus then goes to the
 * column's press, found beside the bar, or to the bar, which must take focus
 * with tabIndex -1.
 */
export function ViewerDetailsPress({
  barRef,
  label,
  facts,
  narrow,
  defaultOpen = false,
}: ViewerBarDetails & { barRef: RefObject<HTMLElement | null> }) {
  // Hold the press, to see when it leaves the bar, and whether it is open.
  const pressRef = useRef<HTMLButtonElement>(null)
  const [open, setOpen] = useState(defaultOpen)
  const onResize = useEffectEvent(() => {
    if (isHidden(pressRef.current)) setOpen(false)
  })

  // Close the popover once the pane folds its press out of the bar.
  useEffect(() => {
    // Watch the bar only while the popover is open.
    const header = barRef.current
    if (!open || !header) return
    onResize()
    const observer = new ResizeObserver(() => onResize())
    observer.observe(header)
    return () => observer.disconnect()
  }, [open, barRef])

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          ref={pressRef}
          type="button"
          aria-label="Details"
          title="Details"
          className={cn(
            pressClass,
            pressTone.default,
            iconPressClass,
            narrow ? 'inline-flex @pane-2xl:hidden' : 'inline-flex',
          )}
        >
          <LuInfo aria-hidden />
        </button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        variant="status"
        aria-label={label}
        onCloseAutoFocus={(e) => {
          if (!isHidden(pressRef.current)) return
          e.preventDefault()
          focusSide(barRef.current)
        }}
      >
        <div className="pb-1">
          <div className="h-viewer-row text-foreground-alt flex items-center px-3 text-xs font-medium select-none">
            {label}
          </div>
          <FactList dense className="ms-3 me-2">
            {facts}
          </FactList>
        </div>
      </PopoverContent>
    </Popover>
  )
}
