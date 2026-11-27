// Graph utility functions for working with quads and IRIs

// keyToIRI wraps a key string in IRI format: <key>.
export function keyToIRI(key: string): string {
  if (!key) {
    return ''
  }
  return `<${key}>`
}

// isIRI reports whether s is in IRI format: <key>.
export function isIRI(s: string | undefined): s is string {
  return !!s && s.length >= 2 && s.startsWith('<') && s.endsWith('>')
}

// iriToKey extracts the key from an IRI string: <key> -> key.
// iriToKey returns an error if the string is not in IRI format.
export function iriToKey(iri: string): string {
  if (!iri) {
    return ''
  }
  if (!isIRI(iri)) {
    throw new Error(`not an IRI: ${iri}`)
  }
  return iri.slice(1, -1)
}

// predToIRI wraps a predicate string in IRI format: <pred>.
export function predToIRI(pred: string): string {
  if (!pred) {
    return ''
  }
  return `<${pred}>`
}

// graphStringEscapes are the escapes a quoted graph string spells.
const graphStringEscapes: Record<string, string> = {
  '\\': '\\\\',
  '"': '\\"',
  '\n': '\\n',
  '\r': '\\r',
  '\t': '\\t',
}

// quoteGraphString spells s as a quoted graph string.
function quoteGraphString(s: string): string {
  return '"' + s.replace(/[\\"\n\r\t]/g, (c) => graphStringEscapes[c]) + '"'
}

// canonicalGraphValue spells a graph quad field as the World stores it, the
// way the World's canonicalGraphValue does: an IRI <x>, a blank node _:x, a
// typed or language string, and otherwise a quoted string, so a bare
// predicate "p" becomes "\"p\"". An empty field stays empty.
export function canonicalGraphValue(v: string): string {
  if (!v) {
    return ''
  }
  if (v.length >= 2 && v.startsWith('"') && v.endsWith('"')) {
    return quoteGraphString(v.slice(1, -1))
  }
  if (v.length > 2) {
    if (v.startsWith('<') && v.endsWith('>')) {
      return v
    }
    if (v.startsWith('_:')) {
      return v
    }
    let i = v.indexOf('"^^<')
    if (i > 0 && v.startsWith('"') && v.endsWith('>')) {
      return quoteGraphString(v.slice(1, i)) + '^^' + v.slice(i + 3)
    }
    i = v.indexOf('"@')
    if (i > 0 && v.startsWith('"') && !v.endsWith('"')) {
      return quoteGraphString(v.slice(1, i)) + '@' + v.slice(i + 2)
    }
  }
  return quoteGraphString(v)
}
