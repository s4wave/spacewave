export const DOWNLOAD_URL_DRAG_FORMAT = 'DownloadURL'

export interface DownloadDragTarget {
  mimeType: string
  filename: string
  url: string
}

interface DownloadDragDataTransfer {
  setData: (format: string, data: string) => void
}

export function isDownloadURLDragSupported(userAgent: string): boolean {
  if (/Firefox\/|FxiOS\//.test(userAgent)) {
    return false
  }
  return /Chrome\/|Chromium\/|CriOS\/|Edg\/|Safari\//.test(userAgent)
}

export function sanitizeDownloadDragFilename(filename: string): string {
  const sanitized = filename.replace(/[:/\\\r\n]+/g, '-').trim()
  return sanitized || 'download'
}

export function buildDownloadURLDragData(
  target: DownloadDragTarget,
  baseHref: string,
): string {
  const mimeType = target.mimeType || 'application/octet-stream'
  const filename = sanitizeDownloadDragFilename(target.filename)
  const url = new URL(target.url, baseHref).toString()
  return `${mimeType}:${filename}:${url}`
}

export function writeDownloadURLDragTarget(
  dataTransfer: DownloadDragDataTransfer,
  target: DownloadDragTarget,
  baseHref: string,
): void {
  dataTransfer.setData(
    DOWNLOAD_URL_DRAG_FORMAT,
    buildDownloadURLDragData(target, baseHref),
  )
}
