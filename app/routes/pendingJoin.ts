const PENDING_JOIN_KEY = 'spacewave-pending-join'

// storePendingJoin saves an invite code to sessionStorage for post-setup pickup.
export function storePendingJoin(code: string) {
  if (code) sessionStorage.setItem(PENDING_JOIN_KEY, code)
}

// consumePendingJoin retrieves and clears a stored invite code.
export function consumePendingJoin(): string | null {
  const code = sessionStorage.getItem(PENDING_JOIN_KEY)
  if (code) sessionStorage.removeItem(PENDING_JOIN_KEY)
  return code
}
