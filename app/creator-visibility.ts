// areExperimentalCreatorsEnabled returns true when the current build should
// expose experimental creator affordances.
export function areExperimentalCreatorsEnabled(
  isDev = !!import.meta.env?.DEV,
): boolean {
  return isDev
}

// isExperimentalCreatorVisible returns true when a creator should be shown for
// the current build mode.
export function isExperimentalCreatorVisible(
  experimental: boolean | undefined,
  isDev = areExperimentalCreatorsEnabled(),
): boolean {
  return !(experimental ?? false) || isDev
}
