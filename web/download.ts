// downloadPemFile triggers a browser download of PEM data.
// Accepts raw bytes or a decoded string. Defaults filename to
// 'spacewave-backup.pem'.
export function downloadPemFile(
  data: Uint8Array | string,
  filename = 'spacewave-backup.pem',
): void {
  const content = typeof data === 'string' ? data : new Uint8Array(data)
  const blob = new Blob([content], { type: 'application/x-pem-file' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

// downloadURL triggers a browser download for a URL.
const maxBlobDownloadBytes = 64 * 1024 * 1024

export async function downloadURL(url: string, filename = ''): Promise<void> {
  if (isCrossOriginURL(url)) {
    clickDownloadLink(url, filename)
    return
  }

  try {
    const response = await fetch(url, { cache: 'no-store' })
    if (!response.ok) {
      clickDownloadLink(url, filename)
      return
    }
    if (isKnownLargeResponse(response)) {
      clickDownloadLink(url, filename)
      return
    }
    await downloadBlobResponse(response, filename, url)
  } catch {
    clickDownloadLink(url, filename)
  }
}

function isCrossOriginURL(url: string): boolean {
  return new URL(url, window.location.href).origin !== window.location.origin
}

function isKnownLargeResponse(response: Response): boolean {
  const contentLength = Number(response.headers.get('content-length') ?? '')
  return Number.isFinite(contentLength) && contentLength > maxBlobDownloadBytes
}

async function downloadBlobResponse(
  response: Response,
  filename: string,
  sourceURL: string,
): Promise<void> {
  const blob = await readBoundedResponseBlob(response)
  if (!blob) {
    clickDownloadLink(sourceURL, filename)
    return
  }
  const objectURL = URL.createObjectURL(blob)
  clickDownloadLink(
    objectURL,
    filename ||
      parseContentDispositionFilename(
        response.headers.get('content-disposition') ?? '',
      ) ||
      '',
  )
  setTimeout(() => URL.revokeObjectURL(objectURL), 10000)
}

async function readBoundedResponseBlob(
  response: Response,
): Promise<Blob | null> {
  if (!response.body) {
    const blob = await response.blob()
    if (blob.size > maxBlobDownloadBytes) {
      return null
    }
    return blob
  }
  const reader = response.body.getReader()
  const chunks: BlobPart[] = []
  let size = 0
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) {
        return new Blob(chunks, {
          type: response.headers.get('content-type') ?? undefined,
        })
      }
      if (value) {
        size += value.byteLength
        if (size > maxBlobDownloadBytes) {
          await reader.cancel()
          return null
        }
        const chunk = new Uint8Array(value.byteLength)
        chunk.set(value)
        chunks.push(chunk.buffer)
      }
    }
  } finally {
    reader.releaseLock()
  }
}

function clickDownloadLink(url: string, filename: string): void {
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

function parseContentDispositionFilename(header: string): string | undefined {
  const utf8Match = header.match(/(?:^|;\s*)filename\*=UTF-8''([^;]+)/i)
  if (utf8Match?.[1]) {
    try {
      return decodeURIComponent(utf8Match[1])
    } catch {
      return utf8Match[1]
    }
  }
  const quotedMatch = header.match(/(?:^|;\s*)filename="([^"]+)"/i)
  if (quotedMatch?.[1]) {
    return quotedMatch[1]
  }
  const plainMatch = header.match(/(?:^|;\s*)filename=([^;]+)/i)
  return plainMatch?.[1]?.trim()
}
