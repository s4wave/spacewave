import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  resourceContent,
  resourceOf,
} from '@aptre/bldr-sdk/hooks/resourceTransitionState.js'
import { useParams } from '@s4wave/web/router/router.js'

import type {
  UnixFSGalleryCandidate,
  UnixFSGalleryDiscoveryState,
} from '@s4wave/app/unixfs/gallery.js'
import {
  GalleryBody,
  galleryPreviewItems,
  type LightboxSource,
} from '@s4wave/app/unixfs/UnixFSGalleryViewer.js'

// scopePath is the folder the fixture's index reads.
const scopePath = '/photos/2026/trip-to-the-coast'

// names are the images under the folder, some long enough to truncate.
const names = [
  'harbor-at-dawn.jpg',
  'lighthouse.png',
  'tide-pools-with-a-very-long-descriptive-file-name.jpg',
  'gulls.webp',
  'dunes.jpg',
  'boardwalk-sunset.jpg',
  'fog.png',
  'cliffs.jpg',
]

// candidates is the index's image list, in path order.
const candidates: UnixFSGalleryCandidate[] = names.map((name) => ({
  path: `${scopePath}/${name}`,
  name,
  label: name,
  mimeType: name.endsWith('.png')
    ? 'image/png'
    : name.endsWith('.webp')
      ? 'image/webp'
      : 'image/jpeg',
}))

// session is a pane with a session and Space to read previews through.
const session: LightboxSource = {
  spaceId: 'space/photos',
  sessionIndex: 1,
  unixfsId: 'photos',
  httpPathPrefix: '',
}

// noSession is a pane with no session, as a detached pane is.
const noSession: LightboxSource = { ...session, sessionIndex: null }

// noSpace is a pane with a session but outside a Space.
const noSpace: LightboxSource = { ...session, spaceId: null }

// noop stands in for a Retry press.
const noop = () => {}

// index builds the index the watch yields.
function index(
  overrides: Partial<UnixFSGalleryDiscoveryState> = {},
): UnixFSGalleryDiscoveryState {
  return {
    scopePath,
    items: candidates,
    errors: [],
    truncated: false,
    ...overrides,
  }
}

// watch is the watch's resource in one state.
function watch(
  value: UnixFSGalleryDiscoveryState | null,
  {
    loading = false,
    error = null as Error | null,
    last = null as UnixFSGalleryDiscoveryState | null,
  } = {},
): Resource<UnixFSGalleryDiscoveryState> {
  return resourceOf({ value, last, loading, error, retry: noop })
}

interface GalleryFixture {
  galleryState: Resource<UnixFSGalleryDiscoveryState>
  source: LightboxSource
}

// fixtures are the states of UnixFSGalleryViewer.tla, as the watch reports them.
const fixtures: Record<string, GalleryFixture> = {
  tiles: { galleryState: watch(index()), source: session },
  waiting: { galleryState: watch(null, { loading: true }), source: session },
  failed: {
    galleryState: watch(null, { error: new Error('stream closed') }),
    source: session,
  },
  again: {
    galleryState: watch(index(), { loading: true }),
    source: session,
  },
  'failed-kept': {
    galleryState: watch(null, {
      error: new Error('stream closed'),
      last: index(),
    }),
    source: session,
  },
  empty: { galleryState: watch(index({ items: [] })), source: session },
  limit: {
    galleryState: watch(index({ truncated: true })),
    source: session,
  },
  issues: {
    galleryState: watch(index({ errors: [`${scopePath}/private`] })),
    source: session,
  },
  'no-session': { galleryState: watch(index()), source: noSession },
  'no-space': { galleryState: watch(index()), source: noSpace },
}

// tileColors tint the stand-in previews.
const tileColors = ['#2f5d7c', '#7c5a2f', '#3d7c2f', '#6a2f7c']

// stillURL is a flat stand-in preview, since the fixture has no session
// server to read the inline URL from.
function stillURL(i: number): string {
  const color = tileColors[i % tileColors.length]
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><rect width="64" height="64" fill="${color}"/></svg>`
  return `data:image/svg+xml,${encodeURIComponent(svg)}`
}

/**
 * GalleryViewerDebug renders the gallery body in one fixture state, with the
 * viewport as the pane under the file browser's bar, for the viewer sweep:
 * tiles, waiting, failed (before the first index, with Retry), again (the
 * tiles while the watch reads again), failed-kept (the last tiles under the
 * watch's failure past its backoff, with Retry), empty, limit (the index stopped at its
 * limit), issues (some folders unreadable), no-session and no-space. An
 * unknown or missing state shows tiles. The body derives its rows from the watch's
 * resource as the viewer does; only the previews are stand-ins.
 */
export function GalleryViewerDebug() {
  // Read the fixture state from the route.
  const { state = 'tiles' } = useParams()
  const fixture = fixtures[state] ?? fixtures.tiles
  const { galleryState, source } = fixture
  const items = galleryPreviewItems(
    resourceContent(galleryState)?.items ?? [],
    source,
  ).map((item, i) =>
    item.previewURL ? { ...item, previewURL: stillURL(i) } : item,
  )

  return (
    <div className="bg-background-primary viewer-pane relative flex h-full w-full flex-col">
      <GalleryBody
        galleryState={galleryState}
        items={items}
        scopePath={resourceContent(galleryState)?.scopePath ?? scopePath}
        onOpen={noop}
      />
    </div>
  )
}
