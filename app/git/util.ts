// formatRelativeTime formats a Unix timestamp as a relative time string.
export function formatRelativeTime(timestamp: bigint | undefined): string {
  if (!timestamp) return ''
  const seconds = Number(timestamp)
  const now = Math.floor(Date.now() / 1000)
  const diff = now - seconds
  if (diff < 60) return 'just now'
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  if (diff < 604800) return `${Math.floor(diff / 86400)}d ago`
  if (diff < 2592000) return `${Math.floor(diff / 604800)}w ago`
  return new Date(seconds * 1000).toLocaleDateString()
}
