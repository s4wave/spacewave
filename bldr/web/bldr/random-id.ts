// randomId generates a random identifier string.
export function randomId(): string {
  return Math.random().toString(36).substring(2, 9)
}
