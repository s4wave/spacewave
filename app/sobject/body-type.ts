// bodyTypeNames maps body type identifiers to human-readable names.
export const bodyTypeNames: Record<string, string> = {
  space: 'Space',
}

// getBodyTypeName returns the human-readable name for a body type.
export function getBodyTypeName(bodyType: string): string {
  return bodyTypeNames[bodyType] ?? bodyType
}
