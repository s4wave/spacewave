import { useDeferredValue, useMemo, useState, type ReactNode } from 'react'
import { LuChevronRight, LuSearch } from 'react-icons/lu'

import type { ControllerInfo } from '@s4wave/sdk/status/status.pb.js'
import { cn } from '@s4wave/web/style/utils.js'

import { Facts } from './Facts.js'
import { formatCount } from './format.js'
import type { DirectiveGroup, PluginView } from './useSystemModel.js'

// RuntimeView selects which runtime listing the inspector shows.
export type RuntimeView = 'plugins' | 'controllers' | 'directives'

// RuntimeInspector lists the plugin host instances, controllers on the bus,
// and active directives grouped by type, with one filter across all three.
export function RuntimeInspector({
  plugins,
  controllers,
  directives,
  directiveCount,
  view,
  onViewChange,
}: {
  plugins: PluginView[] | null
  controllers: ControllerInfo[] | null
  directives: DirectiveGroup[] | null
  directiveCount: number
  view: RuntimeView
  onViewChange: (view: RuntimeView) => void
}) {
  const [filter, setFilter] = useState('')
  const query = useDeferredValue(filter.trim().toLowerCase())

  // Filter each listing by the shared query.
  const visiblePlugins = useMemo(
    () =>
      (plugins ?? []).filter((plugin) =>
        matches(query, plugin.id, plugin.instanceKey, plugin.state),
      ),
    [plugins, query],
  )
  const visibleControllers = useMemo(
    () =>
      (controllers ?? []).filter((controller) =>
        matches(query, controller.id, controller.description),
      ),
    [controllers, query],
  )
  const visibleDirectives = useMemo(
    () =>
      (directives ?? []).filter(
        (group) =>
          matches(query, group.name) ||
          group.idents.some((ident) => matches(query, ident)),
      ),
    [directives, query],
  )

  const tabs: { id: RuntimeView; label: string; count: number | null }[] = [
    { id: 'plugins', label: 'Plugins', count: plugins?.length ?? null },
    {
      id: 'controllers',
      label: 'Controllers',
      count: controllers?.length ?? null,
    },
    {
      id: 'directives',
      label: 'Directives',
      count: directives ? directiveCount : null,
    },
  ]

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <div
          role="tablist"
          aria-label="Runtime listing"
          className="border-foreground/8 bg-background-card/30 flex rounded-md border p-0.5"
        >
          {tabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              role="tab"
              aria-selected={view === tab.id}
              onClick={() => onViewChange(tab.id)}
              className={cn(
                'focus-visible:ring-brand/50 flex items-center gap-1.5 rounded px-2.5 py-1 text-xs transition-colors focus-visible:ring-2 focus-visible:outline-none',
                view === tab.id
                  ? 'bg-foreground/8 text-foreground'
                  : 'text-foreground-alt/60 hover:text-foreground',
              )}
            >
              {tab.label}
              <span className="text-foreground-alt/50 font-mono tabular-nums">
                {tab.count == null ? '…' : formatCount(tab.count)}
              </span>
            </button>
          ))}
        </div>

        <label className="border-foreground/8 bg-background-card/30 focus-within:border-brand/40 flex min-w-48 flex-1 items-center gap-2 rounded-md border px-2.5 py-1.5">
          <LuSearch
            className="text-foreground-alt/50 size-3.5 shrink-0"
            aria-hidden="true"
          />
          <span className="sr-only">Filter runtime listing</span>
          <input
            type="search"
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
            placeholder="Filter by id, name, or state"
            className="text-foreground placeholder:text-foreground-alt/45 min-w-0 flex-1 bg-transparent text-xs outline-none"
          />
        </label>
      </div>

      <div
        role="tabpanel"
        className="border-foreground/8 bg-background-card/30 min-h-0 flex-1 overflow-auto rounded-lg border"
      >
        {view === 'plugins' && (
          <Listing
            loading={!plugins}
            empty={query ? 'No plugins match.' : 'No plugins are loaded.'}
            count={visiblePlugins.length}
          >
            {visiblePlugins.map((plugin) => (
              <PluginRow key={plugin.key} plugin={plugin} />
            ))}
          </Listing>
        )}

        {view === 'controllers' && (
          <Listing
            loading={!controllers}
            empty={
              query ? 'No controllers match.' : 'No controllers are running.'
            }
            count={visibleControllers.length}
          >
            {visibleControllers.map((controller, index) => (
              <li
                key={`${controller.id}:${index}`}
                className="flex items-baseline gap-3 px-4 py-2 text-xs"
              >
                <span className="text-foreground min-w-0 flex-1 truncate font-mono">
                  {controller.id}
                </span>
                {controller.description && (
                  <span className="text-foreground-alt/60 hidden min-w-0 flex-1 truncate @2xl:block">
                    {controller.description}
                  </span>
                )}
                <span className="text-foreground-alt/50 shrink-0 font-mono tabular-nums">
                  {controller.version}
                </span>
              </li>
            ))}
          </Listing>
        )}

        {view === 'directives' && (
          <Listing
            loading={!directives}
            empty={query ? 'No directives match.' : 'No directives are active.'}
            count={visibleDirectives.length}
          >
            {visibleDirectives.map((group) => (
              <DirectiveRow
                key={group.name}
                group={group}
                largest={directives?.[0]?.idents.length ?? 1}
              />
            ))}
          </Listing>
        )}
      </div>
    </div>
  )
}

// matches reports whether any field contains the lowercase query.
function matches(query: string, ...fields: (string | undefined)[]): boolean {
  return !query || fields.some((field) => field?.toLowerCase().includes(query))
}

// Listing renders a list body with its loading and empty states.
function Listing({
  loading,
  empty,
  count,
  children,
}: {
  loading: boolean
  empty: string
  count: number
  children: ReactNode
}) {
  if (loading) {
    return (
      <p className="text-foreground-alt/50 px-4 py-3 text-xs">
        Reading the runtime…
      </p>
    )
  }
  if (!count) {
    return <p className="text-foreground-alt/50 px-4 py-3 text-xs">{empty}</p>
  }
  return <ul className="divide-foreground/6 divide-y">{children}</ul>
}

// pluginStateClass colors the plugin lifecycle: running is healthy and
// requested is still loading.
const pluginStateClass: Record<string, string> = {
  running: 'bg-success/10 text-success',
  requested: 'bg-brand/10 text-brand',
}

// PluginRow is one plugin host instance. It expands to the manifest recovery
// and native package facts retained for the plugin.
function PluginRow({ plugin }: { plugin: PluginView }) {
  const [open, setOpen] = useState(false)
  const manifest = plugin.manifest
  const nativePackage = plugin.nativePackage
  const flagged =
    !!manifest?.quarantinedCandidateCount || !!nativePackage?.lastError

  return (
    <li>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="hover:bg-foreground/[0.03] focus-visible:bg-foreground/[0.04] flex w-full items-center gap-3 px-4 py-2 text-left text-xs outline-none"
      >
        <LuChevronRight
          className={cn(
            'text-foreground-alt/50 size-3.5 shrink-0 transition-transform duration-150',
            open && 'rotate-90',
          )}
          aria-hidden="true"
        />
        <span className="text-foreground min-w-0 flex-1 truncate font-mono">
          {plugin.id || 'unknown'}
          {plugin.instanceKey && (
            <span className="text-foreground-alt/50">
              {' '}
              {plugin.instanceKey}
            </span>
          )}
        </span>
        {flagged && (
          <span className="bg-warning/10 text-warning rounded px-1.5 py-0.5">
            Needs attention
          </span>
        )}
        <span
          className={cn(
            'rounded px-1.5 py-0.5',
            pluginStateClass[plugin.state] ?? 'bg-warning/10 text-warning',
          )}
        >
          {plugin.state}
        </span>
      </button>

      {open && (
        <div className="grid gap-4 px-4 pt-1 pb-3 pl-10 @3xl:grid-cols-2">
          <div>
            <h4 className="text-foreground-alt/70 mb-1.5 text-xs font-medium">
              Manifest
            </h4>
            <Facts
              empty="No manifest recovery facts retained."
              facts={[
                {
                  label: 'Execute manifest',
                  value: manifest?.executeManifestRef,
                  mono: true,
                },
                {
                  label: 'Download manifest',
                  value: manifest?.downloadManifestRef,
                  mono: true,
                },
                {
                  label: 'Skipped',
                  value: countFact(
                    manifest?.skippedCandidateCount,
                    manifest?.skippedCandidateSummary,
                  ),
                },
                {
                  label: 'Ignored',
                  value: countFact(
                    manifest?.ignoredCandidateCount,
                    manifest?.ignoredCandidateSummary,
                  ),
                },
                {
                  label: 'Quarantined',
                  value: countFact(
                    manifest?.quarantinedCandidateCount,
                    manifest?.quarantinedCandidateSummary,
                  ),
                  tone: 'warning',
                },
              ]}
            />
          </div>
          <div>
            <h4 className="text-foreground-alt/70 mb-1.5 text-xs font-medium">
              Native package
            </h4>
            <Facts
              empty="This plugin has no native package."
              facts={
                nativePackage
                  ? [
                      {
                        label: 'Materialized',
                        value: nativePackage.materialized ?? false,
                      },
                      {
                        label: 'Invalidated',
                        value: nativePackage.invalidated ?? false,
                      },
                      {
                        label: 'Last action',
                        value: nativePackage.lastAction,
                      },
                      {
                        label: 'Directory',
                        value: nativePackage.distDir,
                        mono: true,
                      },
                      {
                        label: 'Updated',
                        value: nativePackage.updatedAt,
                        mono: true,
                      },
                      {
                        label: 'Last error',
                        value: nativePackage.lastError,
                        tone: 'error',
                      },
                    ]
                  : []
              }
            />
          </div>
        </div>
      )}
    </li>
  )
}

// countFact formats a candidate count with its summary, or null when zero.
function countFact(count?: number, summary?: string): string | null {
  if (!count) {
    return null
  }
  return summary ? `${count}: ${summary}` : String(count)
}

// DirectiveRow is one directive type with a bar scaled to the largest group.
// It expands to the identity of every active instance.
function DirectiveRow({
  group,
  largest,
}: {
  group: DirectiveGroup
  largest: number
}) {
  const [open, setOpen] = useState(false)

  return (
    <li>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="hover:bg-foreground/[0.03] focus-visible:bg-foreground/[0.04] flex w-full items-center gap-3 px-4 py-2 text-left text-xs outline-none"
      >
        <LuChevronRight
          className={cn(
            'text-foreground-alt/50 size-3.5 shrink-0 transition-transform duration-150',
            open && 'rotate-90',
          )}
          aria-hidden="true"
        />
        <span className="text-foreground min-w-0 flex-1 truncate font-mono">
          {group.name}
        </span>
        <span
          className="bg-foreground/8 hidden h-1 w-24 shrink-0 overflow-hidden rounded-full @xl:block"
          aria-hidden="true"
        >
          <span
            className="bg-foreground-alt/50 progress-width block h-full rounded-full"
            style={{
              '--progress-width': `${(group.idents.length / largest) * 100}%`,
            }}
          />
        </span>
        <span className="text-foreground-alt/70 w-10 shrink-0 text-right font-mono tabular-nums">
          {formatCount(group.idents.length)}
        </span>
      </button>

      {open && (
        <ul className="text-foreground-alt/70 space-y-1 px-4 pt-1 pb-3 pl-10 font-mono text-xs">
          {group.idents.map((ident, index) => (
            <li key={index} className="break-all">
              {ident || '(no identity)'}
            </li>
          ))}
        </ul>
      )}
    </li>
  )
}
