// PAIRING_CODE_INSTRUCTIONS instructs the receiving client, which may use
// Spacewave in a browser or the desktop app.
export const PAIRING_CODE_INSTRUCTIONS = {
  heading: 'Enter this code on your other device',
  hint: 'Open Spacewave on your other device and enter it under Link My Device.',
}

// PAIRING_SERVICE_UNREACHABLE is the human error state for a pairing attempt
// that could not reach the relay.
export const PAIRING_SERVICE_UNREACHABLE =
  'Could not reach the pairing service. Check your connection and try again.'

// PAIRING_CODE_NOT_FOUND is the recovery message for an expired or unknown
// pairing code.
export const PAIRING_CODE_NOT_FOUND =
  'That pairing code was not found or has expired. Check the code and try again.'

// PAIRING_FAILED is the fallback for a relay error without a known code.
export const PAIRING_FAILED =
  'Could not complete pairing. Check the code and try again.'

// pairingErrorMessage maps a raw pairing error into user-facing copy. Relay
// transport failures stay behind the reachability message, known relay error
// codes get recovery copy, and unknown relay responses use one honest fallback.
export function pairingErrorMessage(raw: string | null | undefined): string {
  if (!raw) {
    return PAIRING_SERVICE_UNREACHABLE
  }
  const relayStatus = raw.match(
    /(?:pairing relay returned\s+|(?:get|post) pairing code:\s*)(\d{3})(?=\s|:)/i,
  )
  if (
    /failed to fetch/i.test(raw) ||
    /network(?:error)?|load failed|could not reach|err_network/i.test(raw)
  ) {
    return PAIRING_SERVICE_UNREACHABLE
  }
  if (relayStatus) {
    if (relayStatus[1].startsWith('5')) {
      return PAIRING_SERVICE_UNREACHABLE
    }
    if (/"code"\s*:\s*"not_found"/i.test(raw) || /\bnot_found\s*:/i.test(raw)) {
      return PAIRING_CODE_NOT_FOUND
    }
    return PAIRING_FAILED
  }
  if (/^pairing code conflict, retry with new code$/i.test(raw)) {
    return raw
  }
  return PAIRING_FAILED
}
