import {
  bootDownloadFraction,
  formatBytes,
  type BootDownload,
} from '@aptre/bldr'

import { cn } from '@s4wave/web/style/utils.js'

// BrowserStartupDownloadList renders the live per-asset download breakdown from
// the boot download registry: one labeled progress bar per boot asset (runtime,
// app bundle, and any plugin modules) with streamed bytes and percent. It
// renders nothing until a producer registers a download, so screens that never
// stream bytes stay unchanged. Completed downloads retire from the list so a
// finished byte counter never lingers under later boot phases; failed rows
// stay visible as evidence.
export function BrowserStartupDownloadList({
  downloads,
}: {
  downloads: BootDownload[]
}) {
  const pendingDownloads = downloads.filter(
    (download) => download.state !== 'complete',
  )
  if (pendingDownloads.length === 0) return null
  return (
    <ul
      aria-label="Downloads"
      data-sw-startup-downloads
      className="swb-downloads"
    >
      {pendingDownloads.map((download) => (
        <li key={download.id} data-sw-startup-download={download.id}>
          <BrowserStartupDownloadRow download={download} />
        </li>
      ))}
    </ul>
  )
}

function BrowserStartupDownloadRow({ download }: { download: BootDownload }) {
  const fraction = bootDownloadFraction(download)
  const failed = download.state === 'error'
  const pct =
    fraction === undefined
      ? undefined
      : Math.max(0, Math.min(100, Math.round(fraction * 100)))
  return (
    <div className="swb-dl-row">
      <div className="swb-dl-head">
        <span className="swb-dl-label">{download.label}</span>
        <span
          data-sw-startup-download-detail
          className={cn(
            'swb-mono swb-dl-detail',
            failed && 'swb-dl-detail--error',
          )}
        >
          {downloadDetailText(download)}
        </span>
      </div>
      <div
        className="swb-bar"
        role="progressbar"
        aria-label={download.label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct}
      >
        {pct === undefined && !failed ? (
          <div className="swb-bar-fill swb-bar-fill--indeterminate" />
        ) : (
          <div className="swb-bar-fill" style={{ width: `${pct ?? 0}%` }} />
        )}
      </div>
    </div>
  )
}

// downloadDetailText renders the streamed byte counter, or the error message
// when the download failed. A known total shows "loaded / total"; an unknown
// total shows the bytes seen so far without a faked denominator.
function downloadDetailText(download: BootDownload): string {
  if (download.state === 'error') return download.error ?? 'Failed'
  if (download.total !== undefined) {
    return `${formatBytes(download.loaded, 1)} / ${formatBytes(download.total, 1)}`
  }
  if (download.loaded > 0) return formatBytes(download.loaded, 1)
  return ''
}
