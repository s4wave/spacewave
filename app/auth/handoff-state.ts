import { HandoffRequest } from '@s4wave/core/session/handoff/handoff.pb.js'
import type { Root } from '@s4wave/sdk/root/root.js'

const handoffStorageKey = 'spacewave-auth-handoff-payload'

function base64urlDecode(input: string): Uint8Array {
  let base64 = input.replace(/-/g, '+').replace(/_/g, '/')
  while (base64.length % 4 !== 0) {
    base64 += '='
  }
  const binary = atob(base64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i)
  }
  return bytes
}

export function decodeHandoffRequest(
  payload: string | null | undefined,
): HandoffRequest | null {
  if (!payload) return null
  try {
    return HandoffRequest.fromBinary(base64urlDecode(payload))
  } catch {
    return null
  }
}

export function setStoredHandoffPayload(payload: string) {
  sessionStorage.setItem(handoffStorageKey, payload)
}

export function clearStoredHandoffPayload() {
  sessionStorage.removeItem(handoffStorageKey)
}

export function getStoredHandoffRequest(): HandoffRequest | null {
  return decodeHandoffRequest(sessionStorage.getItem(handoffStorageKey))
}

export function hasStoredHandoffRequest(): boolean {
  return getStoredHandoffRequest() != null
}

async function encryptForHandoffViaSession(
  root: Root,
  sessionIdx: number,
  devicePublicKey: Uint8Array | undefined,
  sessionNonce: string | undefined,
) {
  if (sessionIdx < 1) {
    throw new Error('Invalid session index')
  }
  const result = await root.mountSessionByIdx({ sessionIdx })
  if (!result) {
    throw new Error('Failed to mount session')
  }
  try {
    await result.session.spacewave.encryptForHandoff({
      devicePublicKey,
      sessionNonce,
    })
  } finally {
    result.session.release()
  }
}

export async function completeStoredHandoff(
  root: Root,
  sessionIdx: number,
): Promise<boolean> {
  const request = getStoredHandoffRequest()
  if (!request) return false
  await encryptForHandoffViaSession(
    root,
    sessionIdx,
    request.devicePublicKey,
    request.sessionNonce,
  )
  clearStoredHandoffPayload()
  return true
}
