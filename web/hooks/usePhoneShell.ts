import { useSyncExternalStore } from 'react'

// PHONE_SHELL_QUERY matches the phone shell rules in web/style/app.css.
const PHONE_SHELL_QUERY =
  '(max-width: 640px) and (pointer: coarse), (max-height: 470px) and (hover: none) and (pointer: coarse)'

function subscribePhoneShell(listener: () => void) {
  const media = window.matchMedia(PHONE_SHELL_QUERY)
  media.addEventListener('change', listener)
  return () => media.removeEventListener('change', listener)
}

function getPhoneShell() {
  return window.matchMedia(PHONE_SHELL_QUERY).matches
}

// usePhoneShell reports whether the shell uses its phone layout.
export function usePhoneShell() {
  return useSyncExternalStore(subscribePhoneShell, getPhoneShell, () => false)
}
