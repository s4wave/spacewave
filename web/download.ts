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
export function downloadURL(url: string, filename = ''): void {
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}
