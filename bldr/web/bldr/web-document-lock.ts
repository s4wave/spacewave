// shouldUseWebDocumentLivenessLock returns whether the page can use the
// document-scoped Web Lock for disconnect detection.
export function shouldUseWebDocumentLivenessLock(): boolean {
  return typeof navigator !== 'undefined' && 'locks' in navigator
}
