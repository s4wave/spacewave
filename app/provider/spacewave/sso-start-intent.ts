const SSO_START_INTENT_KEY = 'spacewave-sso-start-provider'
const SSO_RETURN_TO_KEY = 'spacewave-sso-return-to'

type ReturnPath = string & { readonly returnPath: unique symbol }

interface SSOStartIntent {
  provider: string
  returnTo: ReturnPath
}

export interface ConsumedSSOStartIntent {
  authorized: boolean
  returnTo: string
}

let memoryIntent: SSOStartIntent | null = null
let memoryReturnTo = '/login' as ReturnPath

function parseReturnPath(path: string): ReturnPath {
  return (
    path.startsWith('/') && !path.startsWith('/auth/sso/') ? path : '/login'
  ) as ReturnPath
}

function parseIntent(serialized: string | null): SSOStartIntent | null {
  if (!serialized) return null
  try {
    const parsed: unknown = JSON.parse(serialized)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed))
      return null
    const intent = parsed as Record<string, unknown>
    if (typeof intent.provider !== 'string') return null
    if (typeof intent.returnTo !== 'string') return null
    return {
      provider: intent.provider,
      returnTo: parseReturnPath(intent.returnTo),
    }
  } catch {
    return null
  }
}

function readReturnTo(): string {
  try {
    const stored = sessionStorage.getItem(SSO_RETURN_TO_KEY)
    if (stored) return parseReturnPath(stored)
  } catch {
    // Session storage may be unavailable in restricted browser modes.
  }
  return memoryReturnTo
}

function writeReturnTo(returnTo: string): void {
  const normalized = parseReturnPath(returnTo)
  memoryReturnTo = normalized
  try {
    sessionStorage.setItem(SSO_RETURN_TO_KEY, normalized)
  } catch {
    // Session storage may be unavailable in restricted browser modes.
  }
}

export function setSSOStartIntent(provider: string, returnTo: string): void {
  const intent = {
    provider,
    returnTo: parseReturnPath(returnTo),
  }
  memoryIntent = intent
  writeReturnTo(intent.returnTo)
  try {
    sessionStorage.setItem(SSO_START_INTENT_KEY, JSON.stringify(intent))
  } catch {
    // Session storage may be unavailable in restricted browser modes.
  }
}

export function consumeSSOStartIntent(
  provider: string,
): ConsumedSSOStartIntent {
  let stored = memoryIntent
  memoryIntent = null
  try {
    const sessionIntent = parseIntent(
      sessionStorage.getItem(SSO_START_INTENT_KEY),
    )
    if (sessionIntent != null) {
      sessionStorage.removeItem(SSO_START_INTENT_KEY)
      stored = sessionIntent
    }
  } catch {
    // Session storage may be unavailable in restricted browser modes.
  }
  if (stored?.provider === provider) {
    writeReturnTo(stored.returnTo)
    return { authorized: true, returnTo: stored.returnTo }
  }
  return { authorized: false, returnTo: readReturnTo() }
}
