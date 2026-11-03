import { useDeferredValue, useMemo, useState, type ReactNode } from 'react'
import { LuChevronRight, LuSearch } from 'react-icons/lu'

import type { ControllerInfo, StoreInfo } from '@s4wave/sdk/status/status.pb.js'
import { cn } from '@s4wave/web/style/utils.js'

import { DirectiveTreeView } from './DirectiveTreeView.js'
import type { DirectiveTree } from './directiveTree.js'
import { Facts } from './Facts.js'
import { formatCount } from './format.js'
import type { DirectiveGroup, PluginView } from './useSystemModel.js'

// RuntimeView selects which runtime listing the inspector shows.
export type RuntimeView = 'plugins' | 'controllers' | 'directives'

// DirectiveLayout selects how the directives listing is arranged.
// "tree" draws directives under their origins.
// "type" groups them by directive type.
type DirectiveLayout = 'tree' | 'type'

// RuntimeInspector lists plugin instances grouped by the host that runs them,
// controllers on the bus, and active directives under their origins or grouped
// by type, with one filter across all three.
export function RuntimeInspector({
  plugins,
  controllers,
  directives,
  directiveTree,
  directiveCount,
  view,
  onViewChange,
}: {
  plugins: PluginView[] | null
  controllers: ControllerInfo[] | null
  directives: DirectiveGroup[] | null
  directiveTree: DirectiveTree | null
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
        matches(
          query,
          plugin.id,
          plugin.instanceKey,
          plugin.state,
          plugin.host,
        ),
      ),
    [plugins, query],
  )
  const pluginHosts = useMemo(
    () => groupByHost(visiblePlugins),
    [visiblePlugins],
  )
  const visibleControllers = useMemo(
    () =>
      groupControllers(
        (controllers ?? []).filter((controller) =>
          matches(query, controller.id, controller.description),
        ),
      ),
    [controllers, query],
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
            empty={
              query
                ? 'No plugins match.'
                : 'No plugins are running. Open a Space to start its plugins.'
            }
            count={visiblePlugins.length}
          >
            {pluginHosts.map((group) => (
              <PluginHostGroup key={group.spaceId} group={group} />
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
            {visibleControllers.map((group) => (
              <ControllerRow key={group.key} group={group} />
            ))}
          </Listing>
        )}

        {view === 'directives' && (
          <DirectivesPanel
            directives={directives}
            directiveTree={directiveTree}
            query={query}
          />
        )}
      </div>
    </div>
  )
}

// DirectivesPanel lists the active directives under their origins or grouped by type.
function DirectivesPanel({
  directives,
  directiveTree,
  query,
}: {
  directives: DirectiveGroup[] | null
  directiveTree: DirectiveTree | null
  query: string
}) {
  const [layout, setLayout] = useState<DirectiveLayout>('tree')
  const [configKey, setConfigKey] = useState('')
  const visibleDirectives = useMemo(
    () =>
      (directives ?? []).filter(
        (group) =>
          matches(query, group.name) ||
          group.idents.some((ident) => matches(query, ident)),
      ),
    [directives, query],
  )
  const rootKey = directiveTree?.configKeys.includes(configKey) ? configKey : ''

  return (
    <>
      {directiveTree && (
        <DirectiveToolbar
          layout={layout}
          onLayoutChange={setLayout}
          configKeys={directiveTree.configKeys}
          configKey={rootKey}
          onConfigKeyChange={setConfigKey}
          unlinked={directiveTree.unlinked.length}
        />
      )}
      {layout === 'tree' ? (
        <Listing
          loading={!directiveTree}
          empty="No directives are active."
          count={directiveTree?.nodes.size ?? 0}
        >
          {directiveTree && (
            <li>
              <DirectiveTreeView
                tree={directiveTree}
                query={query}
                configKey={rootKey}
              />
            </li>
          )}
        </Listing>
      ) : (
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
    </>
  )
}

// ControllerGroup is one or more running controllers with the same id,
// version, and description.
interface ControllerGroup {
  key: string
  controller: ControllerInfo
  count: number
}

// groupControllers folds identical controllers, such as the bus bridge each
// Space runtime adds, into one row with a count, keeping first-seen order. A
// controller holding a store keeps its own row.
function groupControllers(controllers: ControllerInfo[]): ControllerGroup[] {
  const groups = new Map<string, ControllerGroup>()
  for (const [index, controller] of controllers.entries()) {
    const key =
      `${controller.id}@${controller.version}:${controller.description}` +
      (controller.store ? `#${index}` : '')
    const group = groups.get(key)
    if (group) {
      group.count++
    } else {
      groups.set(key, { key, controller, count: 1 })
    }
  }
  return [...groups.values()]
}

// ControllerRow is one controller, or several identical ones. A controller
// holding a block store it built itself opens into that store's parts.
function ControllerRow({ group }: { group: ControllerGroup }) {
  const [open, setOpen] = useState(false)
  const { controller, count } = group
  const store = controller.store
  const label = (
    <>
      <LuChevronRight
        className={cn(
          'text-foreground-alt/50 size-3.5 shrink-0 transition-transform duration-150',
          open && 'rotate-90',
          !store && 'invisible',
        )}
        aria-hidden="true"
      />
      <span className="text-foreground min-w-0 flex-1 truncate font-mono">
        {controller.id}
        {count > 1 && (
          <span className="text-foreground-alt/50"> ×{formatCount(count)}</span>
        )}
      </span>
      {controller.description && (
        <span className="text-foreground-alt/60 hidden min-w-0 flex-1 truncate @2xl:block">
          {controller.description}
        </span>
      )}
      <span className="text-foreground-alt/50 shrink-0 font-mono tabular-nums">
        {controller.version}
      </span>
    </>
  )

  if (!store) {
    return (
      <li className="flex items-center gap-3 px-4 py-2 text-xs">{label}</li>
    )
  }
  return (
    <li>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="hover:bg-foreground/[0.03] focus-visible:bg-foreground/[0.04] flex w-full items-center gap-3 px-4 py-2 text-left text-xs outline-none"
      >
        {label}
      </button>
      {open && (
        <ul className="pb-1">
          <StoreRow store={store} depth={1} />
        </ul>
      )}
    </li>
  )
}

// StoreRow is one block store and, below it, the stores it is built from.
function StoreRow({ store, depth }: { store: StoreInfo; depth: number }) {
  return (
    <li>
      <div
        style={{ '--tree-depth': depth }}
        className="directive-tree-indent flex items-center gap-3 py-1.5 pr-4 text-xs"
      >
        <span className="text-foreground-alt/50 w-12 shrink-0 truncate">
          {store.label || 'store'}
        </span>
        <span className="text-foreground min-w-0 flex-1 truncate font-mono">
          {store.type || 'unknown'}
        </span>
        {store.id && (
          <span className="text-foreground-alt/60 max-w-80 shrink-0 truncate font-mono">
            {store.id}
          </span>
        )}
      </div>
      {!!store.parts?.length && (
        <ul>
          {store.parts.map((part) => (
            <StoreRow
              key={`${part.label}/${part.type}/${part.id}`}
              store={part}
              depth={depth + 1}
            />
          ))}
        </ul>
      )}
    </li>
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

// PluginHost is the plugins one host runs: the root plugin host or a Space
// runtime.
interface PluginHost {
  spaceId: string
  host: string
  plugins: PluginView[]
}

// groupByHost splits host-ordered plugins into one group per host.
function groupByHost(plugins: PluginView[]): PluginHost[] {
  const groups: PluginHost[] = []
  for (const plugin of plugins) {
    const last = groups.at(-1)
    if (last?.spaceId === plugin.spaceId) {
      last.plugins.push(plugin)
    } else {
      groups.push({
        spaceId: plugin.spaceId,
        host: plugin.host,
        plugins: [plugin],
      })
    }
  }
  return groups
}

// PluginHostGroup is one host's heading followed by its plugin rows.
function PluginHostGroup({ group }: { group: PluginHost }) {
  const running = group.plugins.filter(
    (plugin) => plugin.state === 'running',
  ).length

  return (
    <li>
      <h4 className="bg-foreground/[0.02] text-foreground-alt/70 flex items-baseline gap-3 px-4 py-1.5 text-xs">
        <span className="text-foreground min-w-0 flex-1 truncate font-medium">
          {group.host}
        </span>
        <span className="shrink-0 font-mono tabular-nums">
          {formatCount(running)}/{formatCount(group.plugins.length)} running
        </span>
      </h4>
      <ul className="divide-foreground/6 divide-y">
        {group.plugins.map((plugin) => (
          <PluginRow key={plugin.key} plugin={plugin} />
        ))}
      </ul>
    </li>
  )
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

// DirectiveToolbar switches the directives layout and, in the tree, roots it
// at one ConfigSet key.
function DirectiveToolbar({
  layout,
  onLayoutChange,
  configKeys,
  configKey,
  onConfigKeyChange,
  unlinked,
}: {
  layout: DirectiveLayout
  onLayoutChange: (layout: DirectiveLayout) => void
  configKeys: string[]
  configKey: string
  onConfigKeyChange: (configKey: string) => void
  unlinked: number
}) {
  const layouts: { id: DirectiveLayout; label: string }[] = [
    { id: 'tree', label: 'By origin' },
    { id: 'type', label: 'By type' },
  ]

  return (
    <div className="border-foreground/6 flex flex-wrap items-center gap-3 border-b px-4 py-2 text-xs">
      <div
        role="group"
        aria-label="Directive layout"
        className="border-foreground/8 flex rounded border p-0.5"
      >
        {layouts.map((option) => (
          <button
            key={option.id}
            type="button"
            aria-pressed={layout === option.id}
            onClick={() => onLayoutChange(option.id)}
            className={cn(
              'focus-visible:ring-brand/50 rounded px-2 py-0.5 transition-colors focus-visible:ring-2 focus-visible:outline-none',
              layout === option.id
                ? 'bg-foreground/8 text-foreground'
                : 'text-foreground-alt/60 hover:text-foreground',
            )}
          >
            {option.label}
          </button>
        ))}
      </div>
      {layout === 'tree' && configKeys.length > 0 && (
        <label className="text-foreground-alt/60 flex items-center gap-2">
          Config key
          <select
            value={configKey}
            onChange={(event) => onConfigKeyChange(event.target.value)}
            className="border-foreground/8 bg-background-card/30 text-foreground focus-visible:ring-brand/50 max-w-64 rounded border px-1.5 py-0.5 font-mono focus-visible:ring-2 focus-visible:outline-none"
          >
            <option value="">All</option>
            {configKeys.map((key) => (
              <option key={key} value={key}>
                {key}
              </option>
            ))}
          </select>
        </label>
      )}
      {layout === 'tree' && (
        <span className="text-foreground-alt/50 ml-auto">
          {unlinked
            ? `${formatCount(unlinked)} without an origin`
            : 'Every directive has an origin'}
        </span>
      )}
    </div>
  )
}
