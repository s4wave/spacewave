import { useParams } from '@s4wave/web/router/router.js'

import { DeviceAvailabilityState } from '@s4wave/sdk/device/device.pb.js'
import {
  SyncUploadHold,
  type WatchSyncStatusResponse,
} from '@s4wave/sdk/session/session.pb.js'
import { DriveLinkState, type DriveLink } from '@s4wave/sdk/space/space.pb.js'
import {
  ManageBillingPress,
  SessionSyncStatusPanel,
} from '@s4wave/app/session/SessionSyncStatusButton.js'
import { buildSessionSyncStatusView } from '@s4wave/app/session/SessionSyncStatusContext.js'
import {
  DriveLinksStrip,
  DriveLinksViewProvider,
} from '@s4wave/app/unixfs/DriveLinks.js'
import {
  cloudRow,
  driveLinkRow,
  type DriveLinksView,
} from '@s4wave/app/unixfs/drive-links.js'
import { TextFileFrame } from '@s4wave/app/unixfs/UnixFSTextFileViewer.js'
import {
  deriveTextFileView,
  initialTextFileState,
  reduceTextFile,
  type TextContent,
} from '@s4wave/app/unixfs/text-file.js'

// noop stands in for every press; a fixture holds one state.
const noop = () => {}

const { ONLINE, OFFLINE } = DeviceAvailabilityState

// laptop is Alice's laptop linking ~/Photos to /trips, as its record says.
function laptop(state: DriveLinkState, detail = ''): DriveLink {
  return {
    deviceKey: 'devices/alice-laptop',
    deviceLabel: "Alice's laptop",
    path: 'trips',
    state,
    detail,
  }
}

// here is the laptop's link as the laptop itself sees it.
function here(state: DriveLinkState, files = 0n, bytes = 0n): DriveLink {
  return {
    ...laptop(state),
    local: { folder: '/Users/alice/Photos', files, bytes },
  }
}

function view(
  link: DriveLink,
  availability: DeviceAvailabilityState,
  hold: SyncUploadHold = SyncUploadHold.SyncUploadHold_NONE,
): DriveLinksView {
  return {
    rows: [driveLinkRow({ ...link, availability })],
    cloud: cloudRow(hold),
  }
}

const indexing = DriveLinkState.DriveLinkState_INDEXING
const linked = DriveLinkState.DriveLinkState_LINKED
const rejected = DriveLinkState.DriveLinkState_REJECTED

const content: TextContent = {
  preview: null,
  crlf: false,
  version: new Uint8Array([1]),
  text: 'Day 1: the ferry leaves at nine.\nDay 2: the lighthouse.',
}

// file is the open file in one state: opening, opened, or failed because
// no store this device reached held its blocks.
type FileState = 'opening' | 'opened' | 'unavailable'

function fileView(state: FileState) {
  switch (state) {
    case 'opening':
      return deriveTextFileView(
        { value: null, loading: true, error: null },
        initialTextFileState,
        true,
      )
    case 'opened':
      return deriveTextFileView(
        { value: content, loading: false, error: null },
        reduceTextFile(initialTextFileState, { type: 'read', content }),
        true,
      )
    case 'unavailable':
      return deriveTextFileView(
        {
          value: null,
          loading: false,
          error: new Error('read chunk: block not found'),
        },
        initialTextFileState,
        true,
      )
  }
}

interface Fixture {
  links: DriveLinksView
  file?: FileState
  hold?: SyncUploadHold
}

// fixtures are LinkedFolderStatus.tla's states: the member's row states of
// its chart, named as the chart names them, then the linking Device's own
// view and the sync status button's holds.
const fixtures: Record<string, Fixture> = {
  // Bob, a member, on his Device.
  'folder-indexing': { links: view(laptop(indexing), ONLINE) },
  'no-row': { links: view(laptop(linked), ONLINE) },
  listed: { links: view(laptop(linked), ONLINE) },
  opening: { links: view(laptop(linked), ONLINE), file: 'opening' },
  opened: { links: view(laptop(linked), ONLINE), file: 'opened' },
  unavailable: { links: view(laptop(linked), OFFLINE), file: 'unavailable' },
  'newer-version': {
    links: view(laptop(linked), ONLINE),
    file: 'unavailable',
  },
  // Alice on the linking laptop.
  'here-indexing': {
    links: view(here(indexing, 1284n, 3_520_000_000n), ONLINE),
  },
  'here-rejected': { links: view(here(rejected), ONLINE) },
  'here-held': {
    links: view(
      here(linked),
      ONLINE,
      SyncUploadHold.SyncUploadHold_STORAGE_FULL,
    ),
    hold: SyncUploadHold.SyncUploadHold_STORAGE_FULL,
  },
  'here-spending': {
    links: view(
      here(linked),
      ONLINE,
      SyncUploadHold.SyncUploadHold_SPENDING_LIMIT,
    ),
    hold: SyncUploadHold.SyncUploadHold_SPENDING_LIMIT,
  },
}

/**
 * DriveLinksDebug renders a Drive's linked-folder strip in one state of
 * LinkedFolderStatus.tla, with the open file under it where the state has
 * one, and the sync status panel for a hold: folder-indexing, no-row,
 * listed, opening, opened, unavailable, newer-version, here-indexing,
 * here-rejected, here-held and here-spending. An unknown or missing state
 * shows listed.
 */
export function DriveLinksDebug() {
  const { state = 'listed' } = useParams()
  const fixture = fixtures[state] ?? fixtures.listed
  return (
    <div className="viewer-pane bg-file-back flex h-full w-full flex-col overflow-hidden">
      <DriveLinksViewProvider view={fixture.links}>
        <DriveLinksStrip view={fixture.links} resetting={null} onReset={noop} />
        {fixture.file && (
          <TextFileFrame
            view={fileView(fixture.file)}
            path="/trips/itinerary.txt"
            description="Text, 52 B"
            dispatch={noop}
            onSave={noop}
            retryRead={noop}
            readBack={noop}
          />
        )}
      </DriveLinksViewProvider>
      {fixture.hold !== undefined && (
        <div className="bg-popover mt-auto ml-auto w-80 rounded-md border">
          <SessionSyncStatusPanel
            status={buildSessionSyncStatusView(
              { uploadHold: fixture.hold } as WatchSyncStatusResponse,
              false,
              null,
            )}
            billing={<ManageBillingPress onClick={noop} />}
          />
        </div>
      )}
    </div>
  )
}
