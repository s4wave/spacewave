const INTERACTION_KEY = 'spacewave-has-interacted'

export function hasInteracted(): boolean {
  return localStorage.getItem(INTERACTION_KEY) === 'true'
}

export function markInteracted(): void {
  localStorage.setItem(INTERACTION_KEY, 'true')
}

export function clearInteracted(): void {
  localStorage.removeItem(INTERACTION_KEY)
}
