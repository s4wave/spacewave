import { useCallback, useState, type ReactNode } from 'react'
import {
  LuActivity,
  LuArrowLeft,
  LuBriefcase,
  LuCheck,
  LuCpu,
  LuListTodo,
  LuPlay,
  LuRotateCw,
  LuTriangleAlert,
} from 'react-icons/lu'

import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'

// ForgeViewerDebug renders prototype variants for the Forge ObjectViewer
// surfaces (Task, Job, Pass, Worker, Cluster, Execution, Dashboard). Each
// section shows the current production look first, then candidate variants
// aligned with guides/alpha-ui-design-system.org.
export function ForgeViewerDebug() {
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
          <LuArrowLeft className="h-4 w-4" />
        </button>
        <span className="text-foreground text-sm font-semibold tracking-tight select-none">
          Forge Viewer UI Prototype
        </span>
      </div>

      <div className="flex-1 overflow-auto px-4 py-3">
        <div className="mx-auto max-w-3xl space-y-6">
          <Intro />
          <StateBadgeSection />
          <EntityRowSection />
          <StatTileSection />
          <EmptyStateSection />
          <TabBarSection />
          <ViewerShellSection />
        </div>
      </div>
    </div>
  )
}

// -----------------------------------------------------------------------
// Intro
// -----------------------------------------------------------------------

function Intro() {
  return (
    <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5 backdrop-blur-sm">
      <p className="text-foreground text-sm font-semibold tracking-tight">
        Forge ObjectViewer variant gallery
      </p>
      <p className="text-foreground-alt/70 mt-1 text-xs leading-relaxed">
        Side-by-side variants for the primitives that compose every Forge
        viewer (Task, Job, Pass, Worker, Cluster, Execution, Dashboard). Each
        section shows the current production rendering first, followed by
        candidate variants drawn from the modern token set in
        guides/alpha-ui-design-system.org. Variants exist so we can pick
        one consistent treatment, then propagate it through ForgeViewerShell,
        StateBadge, ForgeEntityList, and the per-type viewers.
      </p>
    </div>
  )
}

// -----------------------------------------------------------------------
// Section wrapper
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
      <div className="border-foreground/6 bg-background/40 rounded-md border p-3">
        {children}
      </div>
    </div>
  )
}

// -----------------------------------------------------------------------
// State badges
// -----------------------------------------------------------------------

type StateTone = 'idle' | 'active' | 'success' | 'warning' | 'error'

interface StateSpec {
  label: string
  tone: StateTone
}

const STATES: StateSpec[] = [
  { label: 'PENDING', tone: 'idle' },
  { label: 'RUNNING', tone: 'active' },
  { label: 'CHECKING', tone: 'active' },
  { label: 'COMPLETE', tone: 'success' },
  { label: 'RETRY', tone: 'error' },
]

function StateBadgeSection() {
  return (
    <Section
      title="State badges"
      description="Forge enums (TaskState, PassState, ExecutionState, JobState) all render through StateBadge. Heavy solid colors fight the rest of the UI; opacity-tinted pills match billing/session badges."
    >
      <Variant
        label="A. Current (web/forge/StateBadge.tsx)"
        note="bg-blue-600 / bg-green-600 / bg-red-600 + white text"
      >
        <div className="flex flex-wrap gap-2">
          {STATES.map((s) => (
            <CurrentBadge key={s.label} {...s} />
          ))}
        </div>
      </Variant>

      <Variant
        label="B. Tinted opacity pill"
        note="border-{tone}/15 bg-{tone}/5 text-{tone}-tone, matches billing status pills"
      >
        <div className="flex flex-wrap gap-2">
          {STATES.map((s) => (
            <TintedBadge key={s.label} {...s} />
          ))}
        </div>
      </Variant>

      <Variant
        label="C. Dot + label minimal"
        note="text-foreground-alt/70 with semantic dot, lowest visual weight"
      >
        <div className="flex flex-wrap gap-3">
          {STATES.map((s) => (
            <DotBadge key={s.label} {...s} />
          ))}
        </div>
      </Variant>
    </Section>
  )
}

function CurrentBadge({ label }: StateSpec) {
  const color =
    label === 'COMPLETE' ? 'bg-green-600'
    : label === 'RUNNING' || label === 'CHECKING' ? 'bg-blue-600'
    : label === 'RETRY' ? 'bg-red-600'
    : 'bg-neutral-600'
  return (
    <span
      className={cn(
        'inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium text-white',
        color,
      )}
    >
      {label}
    </span>
  )
}

const TONE_TINT: Record<StateTone, string> = {
  idle: 'border-foreground/15 bg-foreground/5 text-foreground-alt/70',
  active: 'border-blue-400/20 bg-blue-400/8 text-blue-300',
  success: 'border-emerald-400/20 bg-emerald-400/8 text-emerald-300',
  warning: 'border-amber-400/20 bg-amber-400/8 text-amber-300',
  error: 'border-destructive/30 bg-destructive/8 text-destructive',
}

function TintedBadge({ label, tone }: StateSpec) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full border px-2 py-0.5 text-[0.6rem] font-medium tracking-widest uppercase select-none',
        TONE_TINT[tone],
      )}
    >
      {label}
    </span>
  )
}

const TONE_DOT: Record<StateTone, string> = {
  idle: 'bg-foreground/30',
  active: 'bg-blue-400',
  success: 'bg-emerald-400',
  warning: 'bg-amber-400',
  error: 'bg-destructive',
}

function DotBadge({ label, tone }: StateSpec) {
  return (
    <span className="text-foreground-alt/70 inline-flex items-center gap-1.5 text-[0.6rem] font-medium tracking-widest uppercase select-none">
      <span className={cn('h-1.5 w-1.5 rounded-full', TONE_DOT[tone])} />
      {label}
    </span>
  )
}

// -----------------------------------------------------------------------
// Entity row cards
// -----------------------------------------------------------------------

interface RowSample {
  name: string
  meta: string
  state: StateSpec
}

const ROW_SAMPLES: RowSample[] = [
  {
    name: 'cluster/forge/build',
    meta: '3 workers, peer 12D3KooW...x9q',
    state: { label: 'RUNNING', tone: 'active' },
  },
  {
    name: 'task/clone-repository',
    meta: 'Pass #4, started 22:14',
    state: { label: 'CHECKING', tone: 'active' },
  },
  {
    name: 'task/build-image',
    meta: '2 outputs',
    state: { label: 'COMPLETE', tone: 'success' },
  },
  {
    name: 'task/upload-artifact',
    meta: 'fail: signature mismatch',
    state: { label: 'RETRY', tone: 'error' },
  },
]

function EntityRowSection() {
  return (
    <Section
      title="Entity rows (linked entity lists)"
      description="ForgeEntityList renders Task, Pass, Execution, Worker, Job rows. Current rows use rounded-px-3 with no hover; design-system cards use rounded-lg with /6 to /12 hover."
    >
      <Variant
        label="A. Current (border-foreground/6 bg-background-card/20 rounded px-3 py-2)"
        note="No hover state, tight radius"
      >
        <div className="space-y-2">
          {ROW_SAMPLES.map((s) => (
            <CurrentRow key={s.name} sample={s} />
          ))}
        </div>
      </Variant>

      <Variant
        label="B. Glass card with hover (rounded-lg, /30 -> /50)"
        note="Matches InfoCard radius, design-system hover transitions"
      >
        <div className="space-y-2">
          {ROW_SAMPLES.map((s) => (
            <GlassRow key={s.name} sample={s} />
          ))}
        </div>
      </Variant>

      <Variant
        label="C. Compact dense single-line"
        note="One line, leading icon, brand divider, badge tail"
      >
        <div className="border-foreground/6 bg-background-card/30 divide-foreground/6 divide-y rounded-lg border">
          {ROW_SAMPLES.map((s) => (
            <DenseRow key={s.name} sample={s} />
          ))}
        </div>
      </Variant>
    </Section>
  )
}

function CurrentRow({ sample }: { sample: RowSample }) {
  return (
    <div className="border-foreground/6 bg-background-card/20 flex items-center justify-between rounded border px-3 py-2">
      <div className="min-w-0">
        <div className="text-foreground truncate text-sm font-medium">
          {sample.name}
        </div>
        <div className="text-foreground-alt/50 truncate text-xs">
          {sample.meta}
        </div>
      </div>
      <CurrentBadge {...sample.state} />
    </div>
  )
}

function GlassRow({ sample }: { sample: RowSample }) {
  return (
    <div className="group border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 flex items-center justify-between gap-3 rounded-lg border px-3.5 py-2.5 transition-all duration-150">
      <div className="flex min-w-0 items-center gap-2.5">
        <span className="bg-foreground/5 group-hover:bg-foreground/8 flex h-7 w-7 shrink-0 items-center justify-center rounded-md transition-colors">
          <LuListTodo className="text-foreground-alt/70 h-3.5 w-3.5" />
        </span>
        <div className="min-w-0">
          <div className="text-foreground truncate text-xs font-medium">
            {sample.name}
          </div>
          <div className="text-foreground-alt/50 truncate text-[0.6rem]">
            {sample.meta}
          </div>
        </div>
      </div>
      <TintedBadge {...sample.state} />
    </div>
  )
}

function DenseRow({ sample }: { sample: RowSample }) {
  return (
    <div className="hover:bg-foreground/5 flex items-center gap-2.5 px-3 py-2 transition-colors">
      <LuListTodo className="text-foreground-alt/50 h-3.5 w-3.5 shrink-0" />
      <span className="text-foreground min-w-0 flex-1 truncate text-xs font-medium">
        {sample.name}
      </span>
      <span className="text-foreground-alt/50 hidden truncate text-[0.6rem] sm:block">
        {sample.meta}
      </span>
      <DotBadge {...sample.state} />
    </div>
  )
}

// -----------------------------------------------------------------------
// Stat tiles
// -----------------------------------------------------------------------

function StatTileSection() {
  return (
    <Section
      title="Stat tiles (Job progress, Task counts, Capacity)"
      description="Current viewers stack large numeric tiles using InfoCard. Compact stat groups (billing) and StatCard (session dashboard) are lighter."
    >
      <Variant
        label="A. Current (InfoCard text-2xl)"
        note="As used in ForgeJobViewer / ForgeTaskViewer overview"
      >
        <div className="grid grid-cols-2 gap-3">
          <CurrentStatCard
            icon={<LuListTodo className="h-3.5 w-3.5" />}
            title="Tasks"
            value="6/12"
            detail="50% complete"
          />
          <CurrentStatCard
            icon={<LuPlay className="h-3.5 w-3.5" />}
            title="Passes"
            value="4"
            detail="Current nonce 4"
          />
        </div>
      </Variant>

      <Variant
        label="B. Compact stat group (billing-style)"
        note="Uppercase label, no card per metric"
      >
        <div className="grid grid-cols-2 gap-6">
          <CompactStat label="Tasks" value="6/12" detail="50% complete" />
          <CompactStat label="Passes" value="4" detail="Current nonce 4" />
        </div>
      </Variant>

      <Variant
        label="C. Inline StatCard (session dashboard)"
        note="Brand icon box, tiny label, single-line value"
      >
        <div className="grid grid-cols-2 gap-3">
          <InlineStat
            icon={LuListTodo}
            label="Tasks complete"
            value="6 of 12"
          />
          <InlineStat icon={LuPlay} label="Passes" value="4 (nonce 4)" />
        </div>
      </Variant>
    </Section>
  )
}

function CurrentStatCard({
  icon,
  title,
  value,
  detail,
}: {
  icon: ReactNode
  title: string
  value: string
  detail: string
}) {
  return (
    <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5 backdrop-blur-sm">
      <h3 className="text-foreground mb-3 flex items-center gap-2 text-sm select-none">
        <span className="text-foreground-alt/70">{icon}</span>
        {title}
      </h3>
      <div className="text-foreground text-2xl font-semibold">{value}</div>
      <div className="text-foreground-alt/50 mt-1 text-xs">{detail}</div>
    </div>
  )
}

function CompactStat({
  label,
  value,
  detail,
}: {
  label: string
  value: string
  detail: string
}) {
  return (
    <div className="space-y-1">
      <div className="text-foreground-alt/60 text-xs font-medium tracking-wider uppercase select-none">
        {label}
      </div>
      <div className="text-foreground text-2xl font-semibold tracking-tight">
        {value}
      </div>
      <div className="text-foreground-alt/50 text-[0.6rem]">{detail}</div>
    </div>
  )
}

function InlineStat({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof LuListTodo
  label: string
  value: string
}) {
  return (
    <div className="group border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/60 flex items-center gap-3 rounded-lg border p-3 transition-all duration-150">
      <div className="bg-brand/10 group-hover:bg-brand/15 flex h-9 w-9 shrink-0 items-center justify-center rounded transition-all duration-150">
        <Icon className="text-brand h-4.5 w-4.5" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-foreground-alt text-xs select-none">{label}</p>
        <p className="text-foreground text-sm font-medium select-none">
          {value}
        </p>
      </div>
    </div>
  )
}

// -----------------------------------------------------------------------
// Empty states
// -----------------------------------------------------------------------

function EmptyStateSection() {
  return (
    <Section
      title="Empty states"
      description="Forge viewers use centered hero blocks ('No tasks in job', py-4 text-center). Design system prefers compact single-line states with muted icon + text."
    >
      <Variant
        label="A. Current (text-muted-foreground py-4 text-center)"
        note="Centered, no icon, deprecated text-muted-foreground token"
      >
        <div className="text-muted-foreground py-4 text-center text-xs">
          No tasks in job
        </div>
      </Variant>

      <Variant
        label="B. Compact single-line (design-system)"
        note="Muted icon + text, text-foreground-alt/40"
      >
        <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
          <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
            <LuListTodo className="h-3.5 w-3.5 shrink-0" />
            <span>No tasks linked to this job yet</span>
          </div>
        </div>
      </Variant>
    </Section>
  )
}

// -----------------------------------------------------------------------
// Tab bar
// -----------------------------------------------------------------------

const TABS = ['Overview', 'Tasks', 'Pass History', 'Outputs']

function TabBarSection() {
  const [activeA, setActiveA] = useState(0)
  const [activeB, setActiveB] = useState(0)
  const [activeC, setActiveC] = useState(0)

  return (
    <Section
      title="Tab bar (ForgeViewerShell)"
      description="Current bar uses bg-foreground underline. Brand-tinted underline ties it to the rest of the UI; pill tabs read better when there are 3+ short labels."
    >
      <Variant
        label="A. Current (foreground underline, h-8 row)"
        note="From web/forge/ForgeViewerShell.tsx"
      >
        <CurrentTabs active={activeA} onSelect={setActiveA} />
      </Variant>

      <Variant
        label="B. Brand underline + softer inactive"
        note="bg-brand/80 underline, text-foreground-alt/50 -> /80 inactive"
      >
        <BrandTabs active={activeB} onSelect={setActiveB} />
      </Variant>

      <Variant
        label="C. Pill tabs"
        note="bg-foreground/5 pill row with bg-brand/10 selected"
      >
        <PillTabs active={activeC} onSelect={setActiveC} />
      </Variant>
    </Section>
  )
}

function CurrentTabs({
  active,
  onSelect,
}: {
  active: number
  onSelect: (i: number) => void
}) {
  return (
    <div className="border-foreground/8 -mx-3 -mt-3 mb-0 flex h-8 items-end gap-0 border-b px-3">
      {TABS.map((label, i) => (
        <button
          key={label}
          type="button"
          onClick={() => onSelect(i)}
          className={cn(
            'relative px-3 pt-1 pb-1.5 text-xs font-medium transition-colors',
            i === active ?
              'text-foreground'
            : 'text-foreground/50 hover:text-foreground/70',
          )}
        >
          {label}
          {i === active && (
            <span className="bg-foreground absolute right-1 bottom-0 left-1 h-[2px] rounded-t" />
          )}
        </button>
      ))}
    </div>
  )
}

function BrandTabs({
  active,
  onSelect,
}: {
  active: number
  onSelect: (i: number) => void
}) {
  return (
    <div className="border-foreground/8 -mx-3 -mt-3 mb-0 flex h-9 items-end gap-1 border-b px-3">
      {TABS.map((label, i) => (
        <button
          key={label}
          type="button"
          onClick={() => onSelect(i)}
          className={cn(
            'relative px-3 pt-1.5 pb-2 text-xs font-medium tracking-tight transition-colors',
            i === active ?
              'text-foreground'
            : 'text-foreground-alt/50 hover:text-foreground-alt/80',
          )}
        >
          {label}
          {i === active && (
            <span className="bg-brand/80 absolute right-2 bottom-0 left-2 h-[2px] rounded-t" />
          )}
        </button>
      ))}
    </div>
  )
}

function PillTabs({
  active,
  onSelect,
}: {
  active: number
  onSelect: (i: number) => void
}) {
  return (
    <div className="bg-foreground/5 inline-flex gap-1 rounded-md p-1">
      {TABS.map((label, i) => (
        <button
          key={label}
          type="button"
          onClick={() => onSelect(i)}
          className={cn(
            'rounded px-2.5 py-1 text-xs font-medium transition-colors',
            i === active ?
              'bg-brand/10 text-foreground border-brand/20 border'
            : 'text-foreground-alt/60 hover:text-foreground-alt/90 border border-transparent',
          )}
        >
          {label}
        </button>
      ))}
    </div>
  )
}

// -----------------------------------------------------------------------
// Combined viewer shell
// -----------------------------------------------------------------------

function ViewerShellSection() {
  const [tab, setTab] = useState(0)
  return (
    <Section
      title="Combined viewer mock"
      description="Full ForgeViewerShell layout using the candidate variants: brand underline tabs, tinted state badge, glass entity rows, compact empty state. Compare against ForgeJobViewer in repos/spacewave/app/forge/."
    >
      <div className="border-foreground/6 bg-background-card/30 overflow-hidden rounded-lg border backdrop-blur-sm">
        <div className="border-foreground/8 flex h-9 items-center justify-between border-b px-4">
          <div className="text-foreground flex items-center gap-2 text-sm font-semibold select-none">
            <LuBriefcase className="h-4 w-4" />
            <span className="tracking-tight">Build pipeline</span>
            <TintedBadge label="RUNNING" tone="active" />
          </div>
          <div className="text-foreground-alt/50 text-[0.6rem] tracking-widest uppercase">
            Job
          </div>
        </div>

        <div className="border-foreground/8 flex h-9 items-end gap-1 border-b px-3">
          {TABS.slice(0, 3).map((label, i) => (
            <button
              key={label}
              type="button"
              onClick={() => setTab(i)}
              className={cn(
                'relative px-3 pt-1.5 pb-2 text-xs font-medium tracking-tight transition-colors',
                i === tab ?
                  'text-foreground'
                : 'text-foreground-alt/50 hover:text-foreground-alt/80',
              )}
            >
              {label}
              {i === tab && (
                <span className="bg-brand/80 absolute right-2 bottom-0 left-2 h-[2px] rounded-t" />
              )}
            </button>
          ))}
        </div>

        <div className="space-y-3 px-4 py-3">
          {tab === 0 && (
            <>
              <div className="grid grid-cols-2 gap-3">
                <InlineStat
                  icon={LuListTodo}
                  label="Tasks complete"
                  value="6 of 12"
                />
                <InlineStat icon={LuPlay} label="Passes" value="4" />
              </div>
              <div className="grid grid-cols-3 gap-3">
                <MiniStat
                  icon={LuActivity}
                  label="Active"
                  value="3"
                  tone="active"
                />
                <MiniStat
                  icon={LuCheck}
                  label="Done"
                  value="6"
                  tone="success"
                />
                <MiniStat
                  icon={LuRotateCw}
                  label="Retry"
                  value="1"
                  tone="error"
                />
              </div>
            </>
          )}
          {tab === 1 && (
            <div className="space-y-2">
              {ROW_SAMPLES.map((s) => (
                <GlassRow key={s.name} sample={s} />
              ))}
            </div>
          )}
          {tab === 2 && (
            <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
              <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
                <LuPlay className="h-3.5 w-3.5 shrink-0" />
                <span>No passes recorded yet</span>
              </div>
            </div>
          )}
        </div>

        <div className="border-foreground/8 flex h-10 items-center gap-2 border-t px-4">
          <ShellAction icon={<LuCpu className="h-3.5 w-3.5" />} label="Run" />
          <ShellAction
            icon={<LuTriangleAlert className="h-3.5 w-3.5" />}
            label="Cancel"
            variant="destructive"
          />
        </div>
      </div>
    </Section>
  )
}

function MiniStat({
  icon: Icon,
  label,
  value,
  tone,
}: {
  icon: typeof LuActivity
  label: string
  value: string
  tone: StateTone
}) {
  return (
    <div className="border-foreground/6 bg-background/40 flex items-center gap-2 rounded-md border px-2.5 py-2">
      <span
        className={cn(
          'flex h-6 w-6 shrink-0 items-center justify-center rounded',
          tone === 'success' ? 'bg-emerald-400/10'
          : tone === 'error' ? 'bg-destructive/10'
          : 'bg-blue-400/10',
        )}
      >
        <Icon
          className={cn(
            'h-3 w-3',
            tone === 'success' ? 'text-emerald-300'
            : tone === 'error' ? 'text-destructive'
            : 'text-blue-300',
          )}
        />
      </span>
      <div className="min-w-0">
        <div className="text-foreground-alt/50 text-[0.55rem] tracking-widest uppercase">
          {label}
        </div>
        <div className="text-foreground text-sm font-semibold">{value}</div>
      </div>
    </div>
  )
}

function ShellAction({
  icon,
  label,
  variant,
}: {
  icon: ReactNode
  label: string
  variant?: 'default' | 'destructive'
}) {
  return (
    <button
      type="button"
      className={cn(
        'flex items-center gap-1.5 rounded px-2.5 py-1 text-xs font-medium transition-colors',
        variant === 'destructive' ?
          'text-destructive hover:bg-destructive/10'
        : 'text-foreground/80 hover:bg-foreground/5',
      )}
    >
      {icon}
      {label}
    </button>
  )
}

