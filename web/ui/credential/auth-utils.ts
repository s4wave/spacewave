// methodLabel returns a human-readable label for an auth method type.
export function methodLabel(method: string): string {
  switch (method) {
    case 'password':
      return 'Password'
    case 'pem':
      return 'Backup key (.pem)'
    case 'passkey':
    case 'webauthn':
      return 'Passkey'
    case 'google_sso':
      return 'Google'
    case 'github_sso':
      return 'GitHub'
    default:
      return method
  }
}

// mapAuthError translates raw cloud error codes into user-friendly messages.
export function mapAuthError(msg: string): string {
  if (msg.includes('unknown_keypair')) {
    return 'The selected key is not registered on this account.'
  }
  if (msg.includes('invalid_signature')) {
    return 'Signature verification failed. The password or key file may be incorrect.'
  }
  if (msg.includes('insufficient_signatures')) {
    return 'Not enough valid signatures. Unlock additional auth methods.'
  }
  if (msg.includes('last_method')) {
    return 'Cannot remove the last auth method on this account.'
  }
  return msg
}

// truncatePeerId shortens a peer ID for display.
export function truncatePeerId(peerId: string): string {
  return peerId.length > 16 ?
      peerId.slice(0, 8) + '...' + peerId.slice(-8)
    : peerId
}
