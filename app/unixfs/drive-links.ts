import { DeviceAvailabilityState } from '@s4wave/sdk/device/device.pb.js'
import { DriveLinkState, type DriveLink } from '@s4wave/sdk/space/space.pb.js'
import { SyncUploadHold } from '@s4wave/sdk/session/session.pb.js'
import { formatBytes, formatCount } from '@s4wave/app/system/format.js'

/**
 * DriveLinkRow is one linking Device's line in a Drive's link strip, from
 * LinkedFolderStatus.tla's Hint for the linker and what a member observes:
 *
 * - offline: the Device has no live link, so its files wait for it to wake.
 * - starting: the node runs but has not reported yet.
 * - indexing: the Device indexes the folder; counts are set only on the
 *   linking Device itself, which alone holds them.
 * - linked: the Drive equals the folder.
 * - rejected: the folder's root is on another file system than the one it
 *   was linked on; folder is set on the linking Device, which offers Reset.
 *
 * here is set on the linking Device itself, which holds the folder's files.
 */
export interface DriveLinkRow {
  deviceKey: string
  name: string
  path: string
  state: 'offline' | 'starting' | 'indexing' | 'linked' | 'rejected'
  label: string
  detail: string
  here: boolean
  folder?: string
}

/** DriveLinksView is what a Drive shows of the folders linked into it. */
export interface DriveLinksView {
  rows: DriveLinkRow[]
  // cloud is set in a Space Spacewave Cloud holds, with the line its row
  // shows.
  cloud: { label: string; detail: string; held: boolean } | null
  // othersReached is how many peers besides the linking Devices this session
  // reaches; any may serve the files, as a Device keeping a full copy does.
  othersReached: number
}

/** driveLinkRow derives a linking Device's line from its link. */
export function driveLinkRow(link: DriveLink): DriveLinkRow {
  const base = {
    deviceKey: link.deviceKey ?? '',
    name: link.deviceLabel || 'Device',
    path: '/' + (link.path ?? ''),
    here: !!link.local,
  }
  const local = link.local
  if (link.availability === DeviceAvailabilityState.OFFLINE) {
    return {
      ...base,
      state: 'offline',
      label: 'Offline',
      detail: 'Its files open again when it wakes.',
    }
  }
  switch (link.state ?? DriveLinkState.DriveLinkState_STARTING) {
    case DriveLinkState.DriveLinkState_REJECTED:
      return {
        ...base,
        state: 'rejected',
        label: 'The folder is on another disk now',
        detail: local
          ? 'Reset links the folder where it is now and syncs it again.'
          : 'Its files wait until the Device links the folder again.',
        folder: local?.folder,
      }
    case DriveLinkState.DriveLinkState_INDEXING:
      return {
        ...base,
        state: 'indexing',
        label: 'Indexing the folder',
        detail: local
          ? `${formatCount(Number(local.files ?? 0))} ${local.files === 1n ? 'file' : 'files'}, ${formatBytes(local.bytes)}`
          : '',
      }
    case DriveLinkState.DriveLinkState_LINKED:
      return {
        ...base,
        state: 'linked',
        label: 'Linked',
        detail: watchingNote(link.detail ?? ''),
      }
    default:
      return { ...base, state: 'starting', label: 'Starting', detail: '' }
  }
}

/** watchingNote is the note after a watching status, such as files changing. */
function watchingNote(detail: string): string {
  const i = detail.indexOf('; ')
  if (i < 0) return ''
  const note = detail.slice(i + 2)
  return note.charAt(0).toUpperCase() + note.slice(1) + '.'
}

/** cloudRow is Spacewave Cloud's line, naming a hold on this session's uploads. */
export function cloudRow(hold: SyncUploadHold): DriveLinksView['cloud'] {
  switch (hold) {
    case SyncUploadHold.SyncUploadHold_STORAGE_FULL:
      return {
        label: 'Storage full',
        detail: 'New changes stay on this device.',
        held: true,
      }
    case SyncUploadHold.SyncUploadHold_SPENDING_LIMIT:
      return {
        label: 'Spending limit reached',
        detail: 'New changes stay on this device.',
        held: true,
      }
    default:
      return { label: 'Keeps a copy', detail: '', held: false }
  }
}

/**
 * unavailableNote says that no holder of a linked Drive's files can serve one
 * now, naming the linking Devices, or returns null while one may. It is
 * LinkedFolderStatus.tla's RowHint for a file this Device has not opened,
 * derived from the holders' watched state alone, so it holds before and
 * during a fetch and a fetch that hangs still shows it. A holder may serve
 * while it is this Device, which links the folder itself; a linking Device
 * whose availability is not offline and whose folder is not rejected, an
 * unknown availability counting as reachable; any other peer the session
 * reaches, such as a Device keeping a full copy; or Spacewave Cloud in a
 * cloud-backed Space. HintTruthful asks that the note never show while one
 * of them serves; a fetch that fails while one looks reachable is the
 * viewer's own failure, with Retry.
 */
export function unavailableNote(view: DriveLinksView | null): string | null {
  if (!view || view.cloud || view.othersReached > 0) return null
  const reachable = (r: DriveLinkRow) =>
    r.here || (r.state !== 'offline' && r.state !== 'rejected')
  if (view.rows.some(reachable)) return null
  const names = view.rows.map((r) => r.name)
  return `Unavailable: no holder can serve it now. Held by ${listNames(names)}.`
}

/** listNames joins names as prose: "a", "a and b", "a, b and c". */
function listNames(names: string[]): string {
  if (names.length <= 1) return names[0] ?? 'nobody reachable'
  return names.slice(0, -1).join(', ') + ' and ' + names.at(-1)
}
