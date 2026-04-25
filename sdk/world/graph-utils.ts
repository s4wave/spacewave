// Graph utility functions for working with quads and IRIs

// KeyToIRI wraps a key string in IRI format: <key>
export function keyToIRI(key: string): string {
  if (!key) {
    return ''
  }
  return `<${key}>`
}

// IRIToKey extracts the key from an IRI string: <key> -> key
// Returns error if the string is not in IRI format
export function iriToKey(iri: string): string {
  if (!iri) {
    return ''
  }
  if (!iri.startsWith('<') || !iri.endsWith('>')) {
    throw new Error(`not an IRI: ${iri}`)
  }
  return iri.slice(1, -1)
}

// PredToIRI wraps a predicate string in IRI format: <pred>
export function predToIRI(pred: string): string {
  if (!pred) {
    return ''
  }
  return `<${pred}>`
}
