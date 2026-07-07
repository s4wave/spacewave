// PairingCodeInstructions is the runtime-aware copy for the code-display step.
export interface PairingCodeInstructions {
  heading: string
  hint: string
}

// pairingCodeInstructions returns the heading and hint shown while a device
// displays its pairing code. isDesktop is true when this device runs the native
// desktop app; in that case the other device is where the code is entered, so
// the copy never tells a desktop user to "open the desktop app". The other
// device's runtime is unknown, so it is named generically. From the web the
// linked target is the desktop app, matching the download-desktop flow.
export function pairingCodeInstructions(
  isDesktop: boolean,
): PairingCodeInstructions {
  if (isDesktop) {
    return {
      heading: 'Enter this code on your other device',
      hint: 'Open Spacewave on your other device and enter it under Link My Device.',
    }
  }
  return {
    heading: 'Enter this code in your desktop app',
    hint: 'Open the Spacewave desktop app and enter it under Link My Device.',
  }
}

// PAIRING_SERVICE_UNREACHABLE is the human error state for a pairing attempt
// that could not reach the relay.
export const PAIRING_SERVICE_UNREACHABLE =
  'Could not reach the pairing service. Check your connection and try again.'

// pairingErrorMessage maps a raw pairing error into user-facing copy. Relay
// transport failures (a raw fetch failure or a relay 5xx surfaced as "pairing
// relay returned 5xx") collapse to a single reachability message; other errors,
// which are user-actionable (code conflict, rejected pairing), pass through.
export function pairingErrorMessage(raw: string | null | undefined): string {
  if (!raw) {
    return PAIRING_SERVICE_UNREACHABLE
  }
  if (
    /failed to fetch/i.test(raw) ||
    /pairing relay returned 5\d\d/i.test(raw) ||
    /network(?:error)?|load failed|could not reach|err_network/i.test(raw)
  ) {
    return PAIRING_SERVICE_UNREACHABLE
  }
  return raw
}
