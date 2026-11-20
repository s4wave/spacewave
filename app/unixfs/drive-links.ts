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
 */
export interface DriveLinkRow {
  deviceKey: string
  name: string
  path: string
  state: 'offline' | 'starting' | 'indexing' | 'linked' | 'rejected'
  label: string
  detail: string
  folder?: string
}

/** DriveLinksView is what a Drive shows of the folders linked into it. */
export interface DriveLinksView {
  rows: DriveLinkRow[]
  // cloud is set in a Space Spacewave Cloud holds, with the line its row
  // shows.
  cloud: { label: string; detail: string; held: boolean } | null
}

/** driveLinkRow derives a linking Device's line from its link. */
export function driveLinkRow(link: DriveLink): DriveLinkRow {
  const base = {
    deviceKey: link.deviceKey ?? '',
    name: link.deviceLabel || 'Device',
    path: '/' + (link.path ?? ''),
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
 * unavailableMessage says why a file in a linked Drive could not be read:
 * a linking Device that is online and linked has moved on from the version
 * listed, so a newer one is on its way; otherwise no holder can serve it,
 * and the message names the holders.
 */
export function unavailableMessage(view: DriveLinksView): string {
  if (view.rows.some((r) => r.state === 'linked' || r.state === 'indexing')) {
    return 'A newer version is on its way.'
  }
  const holders = view.rows.map((r) => r.name)
  if (view.cloud) holders.push('Spacewave Cloud')
  return `Unavailable: no holder can serve it now. Held by ${listNames(holders)}.`
}

/** listNames joins names as prose: "a", "a and b", "a, b and c". */
function listNames(names: string[]): string {
  if (names.length <= 1) return names[0] ?? 'nobody reachable'
  return names.slice(0, -1).join(', ') + ' and ' + names.at(-1)
}

/**
 * isBlockUnavailable reports whether a read failed because no store this
 * device reached held a block of the file.
 */
export function isBlockUnavailable(err: Error): boolean {
  return err.message.includes('block not found')
}
