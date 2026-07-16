// isStorageQuotaError reports whether an error describes exhausted browser
// storage.
export function isStorageQuotaError(error: unknown): boolean {
  const message =
    error instanceof Error ? `${error.name}: ${error.message}` : String(error)
  return (
    message.includes('QuotaExceededError') ||
    message.includes('browser storage quota exceeded')
  )
}
