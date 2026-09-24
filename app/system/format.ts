import type { StorageProtectionState } from '@s4wave/app/session/storage/useStorageHealth.js'

// formatBytes formats a byte count with binary units, keeping one decimal
// place below ten units so small changes stay visible.
export function formatBytes(bytes: bigint | number | null | undefined): string {
  const value = Number(bytes ?? 0)
  if (value < 1024) {
    return `${value} B`
  }

  const units = ['KiB', 'MiB', 'GiB', 'TiB']
  const exp = Math.min(
    Math.floor(Math.log(value) / Math.log(1024)),
    units.length,
  )
  const amount = value / 1024 ** exp
  return `${amount >= 10 ? amount.toFixed(0) : amount.toFixed(1)} ${units[exp - 1]}`
}

// formatCount formats a count with locale grouping.
export function formatCount(count: bigint | number | null | undefined): string {
  return Number(count ?? 0).toLocaleString('en-US')
}

// plural returns the count followed by the singular or plural noun.
export function plural(
  count: number,
  singular: string,
  pluralForm = `${singular}s`,
): string {
  return `${formatCount(count)} ${count === 1 ? singular : pluralForm}`
}

// shortId abbreviates a long identifier to its head and tail for compact
// readouts. The full identifier belongs in the inspector.
export function shortId(id: string, keep = 6): string {
  if (id.length <= keep * 2 + 1) {
    return id
  }
  return `${id.slice(0, keep)}…${id.slice(-keep)}`
}

// protectionLabel describes the browser's automatic cleanup protection.
export function protectionLabel(state: StorageProtectionState): string {
  switch (state) {
    case 'checking':
      return 'Checking'
    case 'protected':
      return 'On'
    case 'not-protected':
      return 'Off'
    case 'unavailable':
      return 'Not available in this browser'
  }
}
