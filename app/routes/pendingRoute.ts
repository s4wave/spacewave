const PENDING_ROUTE_KEY = 'spacewave-pending-route'

// BEARER_INVITE_PREFIX marks a join route payload as a bearer invitation.
export const BEARER_INVITE_PREFIX = 'bearer:'

// joinRoute returns the session route that joins a Space through a bearer
// invitation.
export function joinRoute(code: string): string {
  return `join/${BEARER_INVITE_PREFIX}${code}`
}

// inviteRoute returns the session route that redeems a platform invite.
export function inviteRoute(code: string): string {
  return `invite/${code}`
}

// storePendingRoute saves a session route to open once a session exists, so
// the route survives sign-in and account creation.
export function storePendingRoute(route: string, storage = sessionStorage) {
  storage.setItem(PENDING_ROUTE_KEY, route)
}

// consumePendingRoute retrieves and clears the stored session route.
export function consumePendingRoute(storage = sessionStorage): string | null {
  const route = storage.getItem(PENDING_ROUTE_KEY)
  if (route) storage.removeItem(PENDING_ROUTE_KEY)
  return route
}
