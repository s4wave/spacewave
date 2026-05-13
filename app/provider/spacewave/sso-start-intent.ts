const SSO_START_INTENT_KEY = 'spacewave-sso-start-provider'

let memoryIntent: string | null = null

export function setSSOStartIntent(provider: string): void {
  memoryIntent = provider
  try {
    sessionStorage.setItem(SSO_START_INTENT_KEY, provider)
  } catch {
    // Session storage may be unavailable in restricted browser modes.
  }
}

export function consumeSSOStartIntent(provider: string): boolean {
  let stored = memoryIntent
  memoryIntent = null
  try {
    const sessionIntent = sessionStorage.getItem(SSO_START_INTENT_KEY)
    if (sessionIntent != null) {
      sessionStorage.removeItem(SSO_START_INTENT_KEY)
      stored = sessionIntent
    }
    return stored === provider
  } catch {
    return stored === provider
  }
}
