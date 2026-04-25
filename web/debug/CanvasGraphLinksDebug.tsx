import { useCallback } from 'react'
import {
  LuArrowLeft,
  LuEyeOff,
  LuGitBranch,
  LuLock,
  LuLocateFixed,
  LuPlus,
  LuTrash2,
} from 'react-icons/lu'

import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'
import { GraphLinkPill as ProductionGraphLinkPill } from '@s4wave/app/canvas/GraphLinkPill.js'
import type { EphemeralEdge } from '@s4wave/app/canvas/types.js'

type GraphLinkState = 'unloaded' | 'loaded'
type PillVariant = 'compact' | 'balanced' | 'metadata'

interface GraphLinkFixture {
  id: string
  state: GraphLinkState
  predicate: string
  targetKey?: string
  targetLabel: string
  targetType: string
  direction: 'out' | 'in'
  hidden?: boolean
  policy?: 'normal' | 'protected' | 'deletable'
  truncated?: boolean
}

interface PillVariantFixture {
  id: PillVariant
  name: string
  detail: string
}

const primaryFixtures: GraphLinkFixture[] = [
  {
    id: 'source-parent-workdir',
    state: 'unloaded',
    predicate: 'parent',
    targetLabel: 'workdir/main',
    targetType: 'Drive',
    direction: 'out',
    policy: 'normal',
  },
  {
    id: 'repo-worktree-source',
    state: 'loaded',
    predicate: 'git/worktree',
    targetLabel: 'repo/main',
    targetType: 'Git Repo',
    direction: 'in',
    policy: 'normal',
  },
]

const stateFixtures: GraphLinkFixture[] = [
  {
    id: 'hidden-parent',
    state: 'unloaded',
    predicate: 'parent',
    targetLabel: 'archive/notes',
    targetType: 'Drive',
    direction: 'out',
    hidden: true,
    policy: 'normal',
  },
  {
    id: 'truncated-children',
    state: 'unloaded',
    predicate: 'child',
    targetLabel: 'project-assets',
    targetType: 'Folder',
    direction: 'out',
    policy: 'normal',
    truncated: true,
  },
  {
    id: 'protected-type',
    state: 'loaded',
    predicate: '<type>',
    targetLabel: 'types/canvas',
    targetType: 'ObjectType',
    direction: 'out',
    policy: 'protected',
  },
  {
    id: 'deletable-related',
    state: 'loaded',
    predicate: 'relatedTo',
    targetLabel: 'scratch-plan',
    targetType: 'Canvas',
    direction: 'in',
    policy: 'deletable',
  },
]

const variants: PillVariantFixture[] = [
  {
    id: 'compact',
    name: 'Compact',
    detail: 'Small chip. Fast scanning, minimal metadata.',
  },
  {
    id: 'balanced',
    name: 'Balanced',
    detail: 'Selected direction. Compact padding with readable inline context.',
  },
  {
    id: 'metadata',
    name: 'Metadata',
    detail: 'More type context. Higher readability, larger edge footprint.',
  },
]

function GraphNode({
  label,
  detail,
  muted,
}: {
  label: string
  detail: string
  muted?: boolean
}) {
  return (
    <div
      className={cn(
        'border-foreground/10 flex h-24 w-44 flex-col justify-between rounded-lg border p-3.5 shadow-sm',
        muted ? 'bg-background-card/20 border-dashed' : 'bg-background-card/40',
      )}
    >
      <div className="flex items-center gap-2">
        <div className="bg-brand/10 flex h-7 w-7 items-center justify-center rounded-md">
          <LuGitBranch className="text-brand h-3.5 w-3.5" />
        </div>
        <span className="text-foreground text-xs font-semibold">{label}</span>
      </div>
      <span className="text-foreground-alt/50 text-[0.6rem]">{detail}</span>
    </div>
  )
}

function GraphLinkPill({
  fixture,
  variant,
}: {
  fixture: GraphLinkFixture
  variant: PillVariant
}) {
  const loaded = fixture.state === 'loaded'
  const compact = variant === 'compact'
  const balanced = variant === 'balanced'
  const metadata = variant === 'metadata'
  const hidden = fixture.hidden ?? false
  const policy = fixture.policy ?? 'normal'
  if (balanced) {
    return (
      <div data-testid={`graph-link-pill-${variant}-${fixture.state}`}>
        <ProductionGraphLinkPill
          edge={fixtureToEphemeralEdge(fixture)}
          loaded={loaded}
          onPrimary={() => undefined}
          onHide={() => undefined}
        />
      </div>
    )
  }
  return (
    <div
      data-testid={`graph-link-pill-${variant}-${fixture.state}`}
      className={cn(
        'bg-background-card/50 text-foreground flex items-center rounded-md border shadow-lg backdrop-blur-sm',
        compact ? 'gap-1 px-1.5 py-0.5 text-[0.55rem]'
        : balanced ? 'gap-1 px-1.5 py-0.5 text-[0.6rem]'
        : 'gap-1.5 px-2 py-1 text-[0.6rem]',
        metadata && 'px-2.5 py-1.5',
        loaded ? 'border-brand/20' : 'border-foreground/10',
        hidden && 'opacity-55',
      )}
    >
      <span className="text-brand/60 font-medium">{fixture.predicate}</span>
      <span className="text-foreground-alt/30">/</span>
      <span className="max-w-28 truncate font-medium">
        {fixture.targetLabel}
      </span>
      {!compact && (
        <span
          className={cn(
            'text-foreground-alt/50',
            metadata &&
              'border-foreground/8 bg-foreground/5 rounded px-1 py-0.5',
          )}
        >
          {fixture.targetType}
        </span>
      )}
      {fixture.truncated && !compact && (
        <span className="border-warning/20 bg-warning/10 text-warning rounded px-1 py-0.5">
          capped
        </span>
      )}
      {policy === 'protected' && !compact && (
        <span className="border-foreground/8 bg-foreground/5 text-foreground-alt/50 flex items-center gap-1 rounded px-1 py-0.5">
          <LuLock className="h-2.5 w-2.5" />
          protected
        </span>
      )}
      <button
        type="button"
        className="hover:bg-foreground/8 flex items-center gap-1 rounded-md px-1 py-0.5 transition-colors"
        aria-label={`${loaded ? 'Focus' : 'Load'} ${fixture.targetLabel}`}
        disabled={hidden}
      >
        {loaded ?
          <LuLocateFixed className="h-3 w-3" />
        : <LuPlus className="h-3 w-3" />}
        {!compact && (loaded ? 'Focus' : 'Load')}
      </button>
      <button
        type="button"
        className="hover:bg-foreground/8 rounded-md p-0.5 transition-colors"
        aria-label={`Hide ${fixture.predicate} link`}
        disabled={hidden}
      >
        <LuEyeOff className="h-3 w-3" />
      </button>
      {policy === 'deletable' && (
        <button
          type="button"
          className="text-destructive hover:bg-destructive/10 rounded-md p-0.5 transition-colors"
          aria-label={`Delete ${fixture.predicate} link`}
        >
          <LuTrash2 className="h-3 w-3" />
        </button>
      )}
    </div>
  )
}

function fixtureToEphemeralEdge(fixture: GraphLinkFixture): EphemeralEdge {
  const policy = fixture.policy ?? 'normal'
  const targetKey = fixture.targetKey ?? `debug/${fixture.id}`
  return {
    renderKey: fixture.id,
    subject: '<debug/source>',
    predicate: fixture.predicate,
    object: `<${targetKey}>`,
    label: undefined,
    sourceNodeId: 'debug-source',
    sourceObjectKey: 'debug/source',
    sourceGroupKey: 'debug-source',
    sourceGroupIndex: 0,
    sourceGroupOffset: 0,
    outgoingTruncated: fixture.truncated ?? false,
    incomingTruncated: false,
    hiddenCount: fixture.hidden ? 1 : 0,
    direction: fixture.direction,
    linkedObjectKey: targetKey,
    linkedObjectLabel: fixture.targetLabel,
    linkedObjectType: fixture.targetType,
    linkedObjectTypeLabel: fixture.targetType,
    hideable: policy !== 'protected',
    userRemovable: policy === 'deletable',
    protected: policy === 'protected',
    ownerManaged: policy === 'protected',
    targetNodeId: fixture.state === 'loaded' ? 'debug-target' : undefined,
    stubX: fixture.state === 'loaded' ? undefined : 0,
    stubY: fixture.state === 'loaded' ? undefined : 0,
  }
}

function GraphLinkPreview({
  fixture,
  variant,
}: {
  fixture: GraphLinkFixture
  variant: PillVariant
}) {
  const loaded = fixture.state === 'loaded'
  const hidden = fixture.hidden ?? false
  return (
    <div
      data-testid={`graph-link-preview-${variant}-${fixture.state}`}
      className="border-foreground/6 bg-background-card/20 relative h-52 overflow-hidden rounded-lg border"
    >
      <div className="absolute top-14 left-8 z-10">
        <GraphNode label="Canvas source" detail="world object node" />
      </div>

      <div className="absolute top-14 right-8 z-10">
        <GraphNode
          label={fixture.targetLabel}
          detail={
            loaded ? 'already loaded on canvas' : 'available graph target'
          }
          muted={!loaded}
        />
      </div>

      <div
        className={cn(
          'border-foreground-alt/30 absolute top-[104px] right-[13rem] left-[13rem] border-t',
          !loaded && 'border-dashed',
        )}
      />

      <div className="absolute top-[92px] left-1/2 z-20 -translate-x-1/2">
        <GraphLinkPill fixture={fixture} variant={variant} />
      </div>

      <span className="text-foreground-alt/40 absolute bottom-3 left-3 text-[0.55rem] font-medium tracking-widest uppercase">
        {fixture.direction === 'out' ? 'Outgoing' : 'Incoming'} /{' '}
        {hidden ? 'hidden' : fixture.state}
      </span>
      {hidden && (
        <span className="border-foreground/8 bg-background-card/50 text-foreground-alt/50 absolute right-3 bottom-3 rounded-md border px-1.5 py-0.5 text-[0.55rem] font-medium">
          hidden on canvas
        </span>
      )}
    </div>
  )
}

function VariantPreview({ variant }: { variant: PillVariantFixture }) {
  return (
    <section
      data-testid={`graph-link-variant-${variant.id}`}
      className="flex flex-col gap-3"
    >
      <div className="flex items-end justify-between gap-4">
        <div>
          <h2 className="text-foreground text-xs font-medium">
            {variant.name}
          </h2>
          <p className="text-foreground-alt/50 text-[0.6rem]">
            {variant.detail}
          </p>
        </div>
        <span className="border-foreground/8 bg-foreground/5 text-foreground-alt/50 rounded-md border px-1.5 py-0.5 text-[0.55rem] font-medium">
          {variant.id}
        </span>
      </div>
      <div className="grid gap-3">
        {primaryFixtures.map((fixture) => (
          <GraphLinkPreview
            key={fixture.id}
            fixture={fixture}
            variant={variant.id}
          />
        ))}
      </div>
    </section>
  )
}

function StateCoverage() {
  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-foreground text-xs font-medium">State Coverage</h2>
        <p className="text-foreground-alt/50 text-[0.6rem]">
          Balanced pills with edge cases the production graph model must carry.
        </p>
      </div>
      <div className="grid gap-3">
        {stateFixtures.map((fixture) => (
          <GraphLinkPreview
            key={fixture.id}
            fixture={fixture}
            variant="balanced"
          />
        ))}
      </div>
    </section>
  )
}

// CanvasGraphLinksDebug renders static graph-link states for Canvas UI
// iteration before the production graph model is wired.
export function CanvasGraphLinksDebug() {
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  return (
    <div className="bg-background @container flex h-full w-full flex-col overflow-auto">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center gap-2 border-b px-4">
        <button
          type="button"
          onClick={goBack}
          className="text-foreground-alt hover:text-foreground transition-colors"
        >
          <LuArrowLeft className="h-4 w-4" />
        </button>
        <span className="text-foreground text-sm font-semibold tracking-tight select-none">
          Canvas Graph Links
        </span>
      </div>

      <div className="flex-1 overflow-auto px-4 py-3">
        <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
          {variants.map((variant) => (
            <VariantPreview key={variant.id} variant={variant} />
          ))}
          <StateCoverage />
        </div>
      </div>
    </div>
  )
}
