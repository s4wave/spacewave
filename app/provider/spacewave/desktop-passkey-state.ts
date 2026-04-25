// PendingDesktopPasskeyState holds state needed by the native desktop passkey confirm page.
export interface PendingDesktopPasskeyState {
  nonce: string
  username: string
  credentialJson: string
  prfCapable: boolean
  prfSalt: string
  prfOutput: string
}

let pendingState: PendingDesktopPasskeyState | null = null

export function setPendingDesktopPasskeyState(
  state: PendingDesktopPasskeyState,
) {
  pendingState = state
}

export function getPendingDesktopPasskeyState(): PendingDesktopPasskeyState | null {
  return pendingState
}

export function clearPendingDesktopPasskeyState() {
  pendingState = null
}
