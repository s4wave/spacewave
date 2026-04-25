// PendingSSOState holds state needed by the SSO confirm page.
// Used by both desktop (from StartDesktopSSO RPC) and web (from
// /auth/sso/finish/:nonce nonce-result exchange).
export interface PendingSSOState {
  provider: string
  email: string
  nonce: string
  isDesktop: boolean
}

let pendingState: PendingSSOState | null = null

export function setPendingSSOState(state: PendingSSOState) {
  pendingState = state
}

export function getPendingSSOState(): PendingSSOState | null {
  return pendingState
}

export function clearPendingSSOState() {
  pendingState = null
}
