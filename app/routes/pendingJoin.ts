const PENDING_JOIN_KEY = 'spacewave-pending-join'
export const PENDING_BEARER_INVITE_PREFIX = 'bearer:'

// formatPendingJoin marks a route payload as a bearer invitation.
export function formatPendingJoin(code: string): string {
  return `${PENDING_BEARER_INVITE_PREFIX}${code}`
}

// storePendingJoin saves a bearer invitation for post-setup pickup.
export function storePendingJoin(code: string) {
  if (code) sessionStorage.setItem(PENDING_JOIN_KEY, formatPendingJoin(code))
}

// consumePendingJoin retrieves and clears a stored invite handoff.
export function consumePendingJoin(): string | null {
  const code = sessionStorage.getItem(PENDING_JOIN_KEY)
  if (code) sessionStorage.removeItem(PENDING_JOIN_KEY)
  return code
}
