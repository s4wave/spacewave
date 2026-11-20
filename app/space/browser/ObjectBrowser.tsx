import {
  useEffect,
  useEffectEvent,
  useId,
  useMemo,
  useReducer,
  useState,
} from 'react'
import {
  LuArrowLeft,
  LuCopy,
  LuExternalLink,
  LuFolderTree,
  LuList,
  LuNetwork,
  LuSearch,
} from 'react-icons/lu'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import {
  GraphEdgeBucketDirection,
  type TypeCountEntry,
} from '@s4wave/sdk/world/world.pb.js'
import { RootContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useObjectTypeMetadata } from '@s4wave/web/hooks/useObjectTypeMetadata.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'
import {
  getObjectDisplayName,
  getObjectTypeLabel,
  type ObjectTypeMetadataById,
} from '@s4wave/web/space/object-tree.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { formatRelativeTime } from '../../git/util.js'
import { SpaceObjectBrowser } from '../SpaceObjectBrowser.js'
import {
  applyTypeCounts,
  browserHint,
  browserReducer,
  graphSize,
  groupRelations,
  initialBrowserState,
  layoutRelationGraph,
  relationEndKeys,
  shownActions,
  stateSummary,
  visibleTypeCounts,
  type BrowserAction,
  type BrowserEvent,
  type BrowserNav,
  type BrowserScreen,
  type BrowserState,
  type GraphNode,
  type ListNav,
  type RelationGroup,
  type TypeCounts,
} from './object-browser.js'

// objectPage is how many objects a type's list adds per Show more.
const objectPage = 100

// searchPage is how many matches search reads from each type.
const searchPage = 25

// relationLimit bounds the relations read in each direction of one object.
const relationLimit = 200

// searchDelay is how long typing rests before search reads.
const searchDelay = 200

// familyPreview is how many members a family lists before Show all.
const familyPreview = 5

// NameType names a type for a person: singular, or plural for a family.
export type NameType = (typeId: string, plural?: boolean) => string

// ObjectRow is one object of a list: its key, and the title and state its
// type declares, each empty when it declares none.
export interface ObjectRow {
  objectKey: string
  title: string
  state: string
}

// TypeRow is one visible type and its count.
export interface TypeRow {
  typeId: string
  count: number
}

// SearchGroup is one type's matches.
export interface SearchGroup {
  typeId: string
  rows: ObjectRow[]
}

// ObjectDetail is the open object and its relations, or absent when another
// writer deleted it.
export interface ObjectDetail {
  objectKey: string
  typeId: string
  title: string
  // state and time are what the object's type declares, empty or absent for
  // none.
  state: string
  time?: Date
  groups: RelationGroup[]
  // truncated is set when the object has relations past the read limit.
  truncated: boolean
}

// BrowserData is what the current view read.
export type BrowserData =
  | { view: 'types'; types: TypeRow[] }
  | { view: 'objects'; rows: ObjectRow[]; more: boolean }
  | { view: 'search'; groups: SearchGroup[] }
  | { view: 'keys' }
  | { view: 'object'; object: ObjectDetail | null }

// idleWorld stands in for the World while a view is closed: a stream on it
// never opens.
const idleWorld: Resource<IWorldState> = {
  value: null,
  loading: false,
  error: null,
  retry: () => {},
}

// nameTypeWith names a type by its registered label, else by the last
// segment of its ID in words, such as "Forge job" for "glados/forge-job".
export function nameTypeWith(metadataById?: ObjectTypeMetadataById): NameType {
  return (typeId, plural) => {
    if (!typeId) return plural ? 'objects' : 'Object'
    const label = getObjectTypeLabel(typeId, metadataById)
    const name =
      label === typeId ? humanize(typeId.split('/').at(-1) ?? typeId) : label
    return plural ? pluralize(name) : name
  }
}

function humanize(segment: string): string {
  const words = segment.replace(/[-_]+/g, ' ').trim()
  return words.charAt(0).toUpperCase() + words.slice(1)
}

function pluralize(name: string): string {
  if (/(s|x|ch|sh)$/i.test(name)) return name + 'es'
  if (/[^aeiou]y$/i.test(name)) return name.slice(0, -1) + 'ies'
  return name + 's'
}

/**
 * ObjectBrowser is the Space object browser: the Space's types and their
 * counts, a type's objects by title, search, an object's relations as a graph
 * or a grouped list, and the key tree as the expert view. What it shows
 * derives from ObjectBrowser.tla's state.
 */
export function ObjectBrowser() {
  const { spaceState, spaceWorldResource, navigateToObjects } =
    SpaceContainerContext.useContext()
  const rootResource = RootContext.useContext()
  const metadataById = useObjectTypeMetadata(rootResource, spaceState.engineId)
  const nameType = useMemo(() => nameTypeWith(metadataById), [metadataById])
  const [state, dispatch] = useReducer(browserReducer, initialBrowserState)
  const read = useBrowserRead(spaceWorldResource, state.nav)

  return (
    <ObjectBrowserView
      state={state}
      screen={read.screen}
      data={read.data}
      error={read.error}
      nameType={nameType}
      onEvent={dispatch}
      onRetry={read.retry}
      onOpen={(objectKey) => navigateToObjects([objectKey])}
      onMore={read.more}
      keys={<SpaceObjectBrowser embedded={true} />}
    />
  )
}

// useBrowserRead reads what the current view shows. Every view keeps the
// type counts, which give the visible total; each other view opens its own
// read only while it is on screen.
function useBrowserRead(world: Resource<IWorldState>, nav: BrowserNav) {
  const counts = useTypeCounts(world)
  const { types, total } = useMemo(
    () => visibleTypeCounts(counts.value ?? new Map()),
    [counts.value],
  )

  const typeId = nav.view === 'objects' ? nav.typeId : ''
  const { objects, more } = useObjectList(world, typeId)

  const query = nav.view === 'search' ? nav.query.trim() : ''
  const typeIds = useMemo(
    () => types.map((t) => t.typeId ?? '').toSorted(),
    [types],
  )
  const search = useSearch(query ? world : idleWorld, query, typeIds)

  const objectKey = nav.view === 'object' ? nav.objectKey : ''
  const object = useObjectDetail(objectKey ? world : idleWorld, objectKey)

  // The view's own read decides its load; the World's error is the
  // connection.
  const reads: Record<BrowserNav['view'], Resource<unknown>> = {
    types: counts,
    keys: counts,
    objects,
    search,
    object,
  }
  const active = reads[nav.view]
  const load = readLoad(active)
  const screen: BrowserScreen = {
    view: nav.view,
    load,
    conn: world.error ? 'down' : 'up',
    target:
      nav.view === 'object' && object.value?.object === null
        ? 'deleted'
        : 'present',
    count: total,
  }
  const data =
    load === 'ready'
      ? browserData(nav.view, types, objects.value, search.value, object.value)
      : null

  return {
    screen,
    data,
    error: (active.error ?? world.error)?.message,
    retry: () => {
      if (world.error) world.retry()
      else active.retry()
    },
    more,
  }
}

// readLoad is the load state of a view's read.
function readLoad(read: Resource<unknown>): BrowserScreen['load'] {
  if (read.error) return 'failed'
  return read.value === null || read.value === undefined ? 'loading' : 'ready'
}

// browserData shapes the ready read of the current view for the view.
function browserData(
  view: BrowserNav['view'],
  types: readonly TypeCountEntry[],
  objects: { rows: ObjectRow[]; more: boolean } | null | undefined,
  search: SearchGroup[] | null | undefined,
  object: { object: ObjectDetail | null } | null | undefined,
): BrowserData | null {
  switch (view) {
    case 'types':
      return {
        view: 'types',
        types: types.map((t) => ({
          typeId: t.typeId ?? '',
          count: Number(t.count ?? 0n),
        })),
      }
    case 'objects':
      return objects ? { view: 'objects', ...objects } : null
    case 'search':
      return search ? { view: 'search', groups: search } : null
    case 'keys':
      return { view: 'keys' }
    case 'object':
      return object ? { view: 'object', object: object.object } : null
  }
}

// useObjectList reads a type's objects with their titles and states. The list grows a
// page per Show more; another type starts again at one page.
function useObjectList(world: Resource<IWorldState>, typeId: string) {
  const [paging, setPaging] = useState({ typeId, pages: 1 })
  const pages = paging.typeId === typeId ? paging.pages : 1
  const objects = useWorldQuery(
    typeId ? world : idleWorld,
    async (state, signal) => {
      const page = await state.listObjectsWithTypePage(
        typeId,
        { limit: pages * objectPage, declared: true },
        signal,
      )
      const keys = page.objectKeys ?? []
      return {
        rows: keys.map((objectKey, i): ObjectRow => ({
          objectKey,
          title: page.titles?.[i] ?? '',
          state: page.states?.[i] ?? '',
        })),
        more: !!page.more,
      }
    },
    [typeId, pages],
  )
  return { objects, more: () => setPaging({ typeId, pages: pages + 1 }) }
}

// useTypeCounts follows the World's type counts: every count first, then the
// counts each change moves.
function useTypeCounts(world: Resource<IWorldState>): Resource<TypeCounts> {
  return useStreamingResource(
    world,
    async function* (state, signal) {
      let counts: TypeCounts = new Map()
      for await (const resp of state.watchTypeCounts(signal)) {
        counts = applyTypeCounts(counts, resp.typeCounts ?? [])
        yield counts
      }
    },
    [],
    { reconnect: true, keepWhileLoading: true },
  )
}

// useSearch reads the first page of matches of each type at once, showing
// each type's matches as they arrive. The server walks each type only as far
// as the page reads.
function useSearch(
  world: Resource<IWorldState>,
  query: string,
  typeIds: readonly string[],
): Resource<SearchGroup[]> {
  return useStreamingResource(
    world,
    async function* (state, signal) {
      const firstPage = async (typeId: string): Promise<SearchGroup> => {
        for await (const page of state.searchObjectsWithType(
          typeId,
          query,
          searchPage,
          signal,
        )) {
          const keys = page.objectKeys ?? []
          return {
            typeId,
            rows: keys.map((objectKey, i) => ({
              objectKey,
              title: page.titles?.[i] ?? '',
              state: page.states?.[i] ?? '',
            })),
          }
        }
        return { typeId, rows: [] }
      }
      const pending = new Map(
        typeIds.map((typeId) => [typeId, firstPage(typeId)]),
      )
      const groups: SearchGroup[] = []
      while (pending.size) {
        const group = await Promise.race(pending.values())
        pending.delete(group.typeId)
        if (!group.rows.length) continue
        groups.push(group)
        yield groups.toSorted((a, b) => a.typeId.localeCompare(b.typeId))
      }
      if (!groups.length) yield []
    },
    [query, typeIds.join('\n')],
  )
}

// useObjectDetail reads the open object, its relations and what the types
// at both ends declare from one World revision, again on each revision.
function useObjectDetail(world: Resource<IWorldState>, objectKey: string) {
  return useWorldQuery(
    world,
    async (state, signal): Promise<{ object: ObjectDetail | null }> => {
      const [listed, edges] = await Promise.all([
        state.listObjects({ prefix: objectKey, limit: 1 }, signal),
        state.listGraphEdgeBuckets([objectKey], relationLimit, {
          direction: GraphEdgeBucketDirection.BOTH,
          abortSignal: signal,
        }),
      ])
      if (listed.objects?.[0]?.objectKey !== objectKey) return { object: null }

      const bucket = edges.buckets?.[0]
      const metadata = await state.getObjectMetadataBatch(
        [objectKey, ...relationEndKeys(bucket)],
        signal,
        true,
      )
      const byKey = new Map(metadata.map((md) => [md.objectKey ?? '', md]))
      const self = byKey.get(objectKey)
      return {
        object: {
          objectKey,
          typeId: self?.typeId ?? '',
          title: self?.title ?? '',
          state: self?.state ?? '',
          time: self?.time,
          groups: groupRelations(bucket, byKey),
          truncated: !!(bucket?.outgoingTruncated || bucket?.incomingTruncated),
        },
      }
    },
    [objectKey],
  )
}

export interface ObjectBrowserViewProps {
  state: BrowserState
  screen: BrowserScreen
  data: BrowserData | null
  error?: string
  nameType: NameType
  onEvent: (event: BrowserEvent) => void
  onRetry: () => void
  onOpen: (objectKey: string) => void
  onMore: () => void
  // keys is the key tree the expert view shows.
  keys?: React.ReactNode
}

/**
 * ObjectBrowserView draws one state of ObjectBrowser.tla: a press appears
 * only where the spec shows it, and the spec's hint says what the screen
 * waits for or why an action is missing.
 */
export function ObjectBrowserView({
  state,
  screen,
  data,
  error,
  nameType,
  onEvent,
  onRetry,
  onOpen,
  onMore,
  keys,
}: ObjectBrowserViewProps) {
  const shown = shownActions(screen)
  const hint = browserHint(screen)
  const { nav } = state
  const title =
    data?.view === 'object' && data.object ? objectName(data.object) : ''

  return (
    <section aria-label="Objects" className="flex min-w-0 flex-col gap-2">
      <header className="flex min-h-8 items-center gap-2">
        {shown.has('back') ? (
          <Button
            variant="ghost"
            size="sm"
            className="-ml-3 shrink-0"
            onClick={() => onEvent({ type: 'back' })}
          >
            <LuArrowLeft aria-hidden />
            Back
          </Button>
        ) : null}
        <nav
          aria-label="Path"
          className="text-foreground-alt min-w-0 flex-1 truncate text-xs"
        >
          {crumbs(nav, nameType, title).join(' / ')}
        </nav>
        {nav.view === 'types' ? (
          <Button
            variant="outline"
            size="sm"
            className="shrink-0"
            disabled={!shown.has('openKeys')}
            onClick={() => onEvent({ type: 'openKeys' })}
          >
            <LuFolderTree aria-hidden />
            Key tree
          </Button>
        ) : null}
      </header>
      {nav.view === 'types' || nav.view === 'search' ? (
        <BrowserSearch
          key="search"
          query={nav.view === 'search' ? nav.query : ''}
          enabled={shown.has('search')}
          onSearch={(query) => onEvent({ type: 'search', query })}
        />
      ) : null}
      {hint ? (
        <BrowserStatus
          hint={hint}
          error={screen.load === 'failed' ? error : undefined}
          retry={shown.has('retry') ? onRetry : undefined}
        />
      ) : null}
      {data ? (
        <BrowserBody
          data={data}
          state={state}
          shown={shown}
          nameType={nameType}
          onEvent={onEvent}
          onOpen={onOpen}
          onMore={onMore}
          keys={keys}
        />
      ) : null}
    </section>
  )
}

// crumbs names the path to the view: the types, the list, then the object.
function crumbs(nav: BrowserNav, nameType: NameType, title: string): string[] {
  const list = (l: ListNav) =>
    l.view === 'objects'
      ? nameType(l.typeId, true)
      : l.view === 'search'
        ? 'Search "' + l.query.trim() + '"'
        : 'Key tree'
  switch (nav.view) {
    case 'types':
      return ['Types']
    case 'object':
      return [
        'Types',
        list(nav.list),
        title || getObjectDisplayName(nav.objectKey),
      ]
    default:
      return ['Types', list(nav)]
  }
}

// objectMeta is the open object's state and age, as its type declares
// them, such as "running, 2h ago", or empty when it declares neither.
function objectMeta(o: { state: string; time?: Date }): string {
  const age = o.time
    ? formatRelativeTime(BigInt(Math.floor(o.time.getTime() / 1000)))
    : ''
  return [o.state, age].filter(Boolean).join(', ')
}

// objectName is the name an object shows: its title, else its key's name.
function objectName(o: { objectKey: string; title: string }): string {
  return o.title || getObjectDisplayName(o.objectKey)
}

// BrowserSearch is the search field. Typing rests briefly before it searches;
// clearing it returns to the types. A query changed from outside, as by Back,
// replaces the draft.
function BrowserSearch({
  query,
  enabled,
  onSearch,
}: {
  query: string
  enabled: boolean
  onSearch: (query: string) => void
}) {
  const [draft, setDraft] = useState(query)
  const [synced, setSynced] = useState(query)
  if (synced !== query) {
    setSynced(query)
    setDraft(query)
  }
  const id = useId()
  const search = useEffectEvent((next: string) => onSearch(next))
  useEffect(() => {
    if (draft === query) return
    const timer = setTimeout(() => search(draft), searchDelay)
    return () => clearTimeout(timer)
  }, [draft, query])

  return (
    <div className="border-foreground/10 bg-background-card/40 focus-within:border-brand/40 flex h-8 items-center gap-2 rounded-md border px-2">
      <LuSearch aria-hidden className="text-foreground-alt size-3.5 shrink-0" />
      <label htmlFor={id} className="sr-only">
        Search objects
      </label>
      <input
        id={id}
        type="search"
        value={draft}
        disabled={!enabled}
        placeholder="Search by title or key"
        onChange={(e) => setDraft(e.target.value)}
        className="text-foreground placeholder:text-foreground-alt/60 h-full min-w-0 flex-1 bg-transparent text-xs outline-none disabled:opacity-50"
      />
    </div>
  )
}

// BrowserStatus says what the screen waits for, or why a read failed with
// Retry when the connection is up.
function BrowserStatus({
  hint,
  error,
  retry,
}: {
  hint: string
  error?: string
  retry?: () => void
}) {
  const failed = !!retry || !!error
  return (
    <div
      role={failed ? 'alert' : 'status'}
      className={cn(
        'flex min-h-8 items-center gap-2 rounded-md px-2 text-xs',
        failed ? 'bg-destructive/8 text-foreground' : 'text-foreground-alt',
      )}
    >
      <span className="min-w-0 flex-1 truncate">
        {error ? hint + ': ' + error : hint}
      </span>
      {retry ? (
        <Button
          variant="outline"
          size="xs"
          className="h-6 shrink-0"
          onClick={retry}
        >
          Retry
        </Button>
      ) : null}
    </div>
  )
}

function BrowserBody({
  data,
  state,
  shown,
  nameType,
  onEvent,
  onOpen,
  onMore,
  keys,
}: {
  data: BrowserData
  state: BrowserState
  shown: ReadonlySet<BrowserAction>
  nameType: NameType
  onEvent: (event: BrowserEvent) => void
  onOpen: (objectKey: string) => void
  onMore: () => void
  keys?: React.ReactNode
}) {
  const openObject = (objectKey: string) =>
    onEvent({ type: 'openObject', objectKey })
  switch (data.view) {
    case 'types':
      return (
        <ul aria-label="Types" className="flex flex-col">
          {data.types
            .toSorted((a, b) =>
              nameType(a.typeId).localeCompare(nameType(b.typeId)),
            )
            .map((t) => (
              <li key={t.typeId}>
                <RowButton
                  disabled={!shown.has('openType')}
                  onClick={() =>
                    onEvent({ type: 'openType', typeId: t.typeId })
                  }
                  title={nameType(t.typeId, t.count !== 1)}
                  detail={String(t.count)}
                />
              </li>
            ))}
        </ul>
      )
    case 'objects':
      return (
        <div className="flex flex-col gap-1">
          <ObjectRows
            rows={data.rows}
            enabled={shown.has('openObject')}
            onOpen={openObject}
          />
          {data.rows.length === 0 ? (
            <p className="text-foreground-alt px-2 text-xs">
              No objects of this type
            </p>
          ) : null}
          {data.more ? (
            <Button
              variant="ghost"
              size="sm"
              className="self-start"
              onClick={onMore}
            >
              Show more
            </Button>
          ) : null}
        </div>
      )
    case 'search':
      return (
        <div className="flex flex-col gap-3">
          {data.groups.map((g) => (
            <section
              key={g.typeId}
              aria-label={nameType(g.typeId, true)}
              className="flex flex-col"
            >
              <h3 className="text-foreground-alt px-2 text-xs font-medium">
                {nameType(g.typeId, true)}
              </h3>
              <ObjectRows
                rows={g.rows}
                enabled={shown.has('openObject')}
                onOpen={openObject}
              />
            </section>
          ))}
          {data.groups.length === 0 ? (
            <p className="text-foreground-alt px-2 text-xs">No matches</p>
          ) : null}
        </div>
      )
    case 'keys':
      return <>{keys}</>
    case 'object':
      return data.object ? (
        <ObjectView
          object={data.object}
          rel={state.rel}
          shown={shown}
          nameType={nameType}
          onEvent={onEvent}
          onOpen={onOpen}
        />
      ) : null
  }
}

// RowButton is one row of a list: a title, then at the end the object's type
// chip, a detail such as its key, and its state.
function RowButton({
  title,
  chip,
  detail,
  mono,
  state,
  disabled,
  onClick,
}: {
  title: string
  chip?: string
  detail?: string
  mono?: boolean
  state?: string
  disabled?: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className="hover:bg-foreground/5 focus-visible:ring-ring flex min-h-8 w-full items-center gap-3 rounded-md px-2 text-left text-xs outline-none focus-visible:ring-1 disabled:cursor-default disabled:hover:bg-transparent"
    >
      <span className="text-foreground min-w-0 flex-1 truncate">{title}</span>
      {chip ? <TypeChip name={chip} /> : null}
      {detail ? (
        <span
          className={cn(
            'text-foreground-alt shrink-0 truncate',
            mono && 'max-w-1/2 font-mono',
          )}
        >
          {detail}
        </span>
      ) : null}
      {state ? (
        <span className="text-text-muted shrink-0 truncate">{state}</span>
      ) : null}
    </button>
  )
}

// TypeChip names an object's type in a violet outline.
function TypeChip({ name }: { name: string }) {
  return (
    <span className="border-violet/35 text-violet shrink-0 truncate rounded border px-2 text-xs leading-4">
      {name}
    </span>
  )
}

function ObjectRows({
  rows,
  enabled,
  onOpen,
}: {
  rows: readonly ObjectRow[]
  enabled: boolean
  onOpen: (objectKey: string) => void
}) {
  return (
    <ul className="flex flex-col">
      {rows.map((row) => (
        <li key={row.objectKey}>
          <RowButton
            title={objectName(row)}
            detail={row.title ? row.objectKey : undefined}
            mono
            state={row.state}
            disabled={!enabled}
            onClick={() => onOpen(row.objectKey)}
          />
        </li>
      ))}
    </ul>
  )
}

// ObjectView is the open object: its name, type and key, Open, Copy key, the
// Graph or List switch, and its relations drawn the chosen way.
function ObjectView({
  object,
  rel,
  shown,
  nameType,
  onEvent,
  onOpen,
}: {
  object: ObjectDetail
  rel: BrowserState['rel']
  shown: ReadonlySet<BrowserAction>
  nameType: NameType
  onEvent: (event: BrowserEvent) => void
  onOpen: (objectKey: string) => void
}) {
  const follow = shown.has('follow')
    ? (objectKey: string) => onEvent({ type: 'openObject', objectKey })
    : undefined
  const toList = () => {
    if (rel === 'graph') onEvent({ type: 'toggleRelations' })
  }
  const meta = objectMeta(object)
  const copyKey = () => {
    void navigator.clipboard.writeText(object.objectKey).then(
      () => toast.success('Key copied'),
      () => toast.error('Could not copy the key'),
    )
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <h2 className="text-foreground min-w-0 text-sm font-medium break-words">
            {objectName(object)}
          </h2>
          <TypeChip name={nameType(object.typeId)} />
        </div>
        <div className="text-text-muted flex min-w-0 items-baseline gap-4 text-xs">
          {meta ? <span className="shrink-0">{meta}</span> : null}
          <span className="truncate font-mono">{object.objectKey}</span>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {shown.has('open') ? (
          <Button
            variant="brandOutline"
            size="sm"
            onClick={() => onOpen(object.objectKey)}
          >
            <LuExternalLink aria-hidden />
            Open
          </Button>
        ) : null}
        {shown.has('copyKey') ? (
          <Button variant="outline" size="sm" onClick={copyKey}>
            <LuCopy aria-hidden />
            Copy key
          </Button>
        ) : null}
        {shown.has('toggleRelations') ? (
          <div
            role="group"
            aria-label="Relations view"
            className="border-foreground/10 ml-auto flex rounded-md border p-0.5"
          >
            {(['graph', 'list'] as const).map((v) => (
              <Button
                key={v}
                variant={rel === v ? 'selectedBrand' : 'ghost'}
                size="xs"
                aria-pressed={rel === v}
                onClick={() => {
                  if (rel !== v) onEvent({ type: 'toggleRelations' })
                }}
              >
                {v === 'graph' ? (
                  <LuNetwork aria-hidden />
                ) : (
                  <LuList aria-hidden />
                )}
                {v === 'graph' ? 'Graph' : 'List'}
              </Button>
            ))}
          </div>
        ) : null}
      </div>
      {object.groups.length === 0 ? (
        <p className="text-foreground-alt text-xs">No relations</p>
      ) : rel === 'graph' ? (
        <RelationGraph
          object={object}
          nameType={nameType}
          follow={follow}
          toList={toList}
        />
      ) : (
        <RelationList object={object} nameType={nameType} follow={follow} />
      )}
      {object.truncated ? (
        <p className="text-foreground-alt text-xs">
          Showing the first {relationLimit} relations each way
        </p>
      ) : null}
    </div>
  )
}

// RelationGraph draws the object at the center, what it points to above and
// what points to it below. A single object opens on a press; a family, and
// the relations past what the graph draws, open the list.
function RelationGraph({
  object,
  nameType,
  follow,
  toList,
}: {
  object: ObjectDetail
  nameType: NameType
  follow?: (objectKey: string) => void
  toList: () => void
}) {
  const markerId = 'rel-arrow-' + useId().replace(/[^a-zA-Z0-9]/g, '')
  const nodes = layoutRelationGraph(
    { title: objectName(object), subtitle: nameType(object.typeId) },
    object.groups,
    nameType,
  )
  const center = nodes[0]

  return (
    <div className="flex flex-col gap-2">
      <div className="overflow-x-auto">
        <svg
          viewBox={'0 0 ' + graphSize.width + ' ' + graphSize.height}
          width={graphSize.width}
          height={graphSize.height}
          role="group"
          aria-label={'Relations of ' + objectName(object)}
          className="mx-auto block max-w-none"
        >
          <defs>
            <marker
              id={markerId}
              viewBox="0 0 10 10"
              refX="9"
              refY="5"
              markerWidth="7"
              markerHeight="7"
              orient="auto-start-reverse"
            >
              <path d="M0 0L10 5L0 10z" className="fill-foreground/50" />
            </marker>
          </defs>
          {nodes.slice(1).map((node) => (
            <GraphEdge
              key={node.id}
              node={node}
              center={center}
              markerId={markerId}
            />
          ))}
          {nodes.map((node) => (
            <GraphBox
              key={node.id}
              node={node}
              onPress={
                node.kind === 'center'
                  ? undefined
                  : node.kind === 'object'
                    ? follow && node.objectKey
                      ? () => follow(node.objectKey ?? '')
                      : undefined
                    : toList
              }
            />
          ))}
        </svg>
      </div>
      <p className="text-foreground-alt max-w-prose text-xs">
        Dashed boxes are families, which open as a list. Arrows above point out
        of this object; arrows below point into it.
      </p>
    </div>
  )
}

// nodeHeight is a box's height: the center is taller.
function nodeHeight(node: GraphNode): number {
  return node.kind === 'center' ? graphSize.centerHeight : graphSize.nodeHeight
}

// boxEdge is where the line from (x, y) toward the node's center meets the
// node's border.
function boxEdge(
  node: GraphNode,
  x: number,
  y: number,
): { x: number; y: number } {
  const dx = x - node.x
  const dy = y - node.y
  const hw = node.width / 2
  const hh = nodeHeight(node) / 2
  const t = Math.min(
    dx ? hw / Math.abs(dx) : Infinity,
    dy ? hh / Math.abs(dy) : Infinity,
    1,
  )
  return { x: node.x + dx * t, y: node.y + dy * t }
}

function GraphEdge({
  node,
  center,
  markerId,
}: {
  node: GraphNode
  center: GraphNode
  markerId: string
}) {
  const from = boxEdge(center, node.x, node.y)
  const to = boxEdge(node, center.x, center.y)
  const [a, b] = node.edge?.direction === 'in' ? [to, from] : [from, to]
  return (
    <g>
      <path
        d={'M' + a.x + ' ' + a.y + ' L' + b.x + ' ' + b.y}
        className="stroke-foreground/35 fill-none"
        strokeWidth={1.25}
        markerEnd={'url(#' + markerId + ')'}
      />
      {node.edge?.label ? (
        <text
          x={node.edge.x}
          y={node.edge.y}
          textAnchor="middle"
          dominantBaseline="middle"
          className="fill-text-muted stroke-background text-xs"
          strokeWidth={4}
          paintOrder="stroke"
        >
          {node.edge.label}
        </text>
      ) : null}
    </g>
  )
}

function GraphBox({
  node,
  onPress,
}: {
  node: GraphNode
  onPress?: () => void
}) {
  const h = nodeHeight(node)
  const label = node.title + ', ' + node.subtitle
  return (
    <g
      role={onPress ? 'button' : undefined}
      tabIndex={onPress ? 0 : undefined}
      aria-label={onPress ? label : undefined}
      onClick={onPress}
      onKeyDown={(e) => {
        if (!onPress || (e.key !== 'Enter' && e.key !== ' ')) return
        e.preventDefault()
        onPress()
      }}
      className={cn('group outline-none', onPress && 'cursor-pointer')}
    >
      <rect
        x={node.x - node.width / 2}
        y={node.y - h / 2}
        width={node.width}
        height={h}
        rx={node.kind === 'center' ? 9 : 8}
        strokeDasharray={
          node.kind === 'family' || node.kind === 'more' ? '5 4' : undefined
        }
        className={cn(
          'fill-background-card',
          node.kind === 'center' ? 'stroke-brand' : 'stroke-border',
          onPress &&
            'group-hover:stroke-brand/70 group-focus-visible:stroke-focus-ring',
        )}
        strokeWidth={node.kind === 'center' ? 1.5 : 1}
      />
      <text
        x={node.x}
        y={node.y - 4}
        textAnchor="middle"
        className="fill-foreground text-header"
      >
        {node.title}
      </text>
      <text
        x={node.x}
        y={node.y + 13}
        textAnchor="middle"
        className="fill-violet text-xs"
      >
        {node.subtitle}
      </text>
    </g>
  )
}

// RelationList lists the relations grouped by predicate and direction, each
// related object on its line. A family lists its first members and Show all.
function RelationList({
  object,
  nameType,
  follow,
}: {
  object: ObjectDetail
  nameType: NameType
  follow?: (objectKey: string) => void
}) {
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set())
  return (
    <div className="flex flex-col gap-3">
      {object.groups.map((g) => {
        const all = expanded.has(g.id) || g.members.length <= familyPreview + 1
        const members = all ? g.members : g.members.slice(0, familyPreview)
        return (
          <section key={g.id} aria-label={g.label} className="flex flex-col">
            <h3 className="text-text-muted flex items-baseline gap-2 px-2 pb-0.5 text-xs tracking-wide uppercase">
              <span>{g.label}</span>
              {g.members.length > 1 ? (
                <span className="truncate">
                  {[
                    g.members.length + ' ' + nameType(g.typeId, true),
                    stateSummary(g.members),
                  ]
                    .filter(Boolean)
                    .join(', ')}
                </span>
              ) : null}
              <span className="text-warning ml-auto shrink-0 tracking-normal normal-case">
                {g.direction === 'out' ? 'outgoing' : 'incoming'}
              </span>
            </h3>
            <ul className="flex flex-col">
              {members.map((m) => (
                <li key={m.objectKey}>
                  <RowButton
                    title={objectName(m)}
                    chip={nameType(m.typeId)}
                    state={m.state}
                    disabled={!follow}
                    onClick={() => follow?.(m.objectKey)}
                  />
                </li>
              ))}
            </ul>
            {!all ? (
              <Button
                variant="ghost"
                size="xs"
                className="h-6 self-start"
                onClick={() => setExpanded((prev) => new Set(prev).add(g.id))}
              >
                Show all {g.members.length}
              </Button>
            ) : null}
          </section>
        )
      })}
    </div>
  )
}
