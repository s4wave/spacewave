// SuggestContext is what the editor can suggest at the caret of a PromQL
// expression.
export type SuggestContext =
  // The caret ends a metric name, or a word outside any selector.
  | { kind: 'metric'; prefix: string; start: number }
  // The caret ends a label name inside the braces of metric's selector.
  | { kind: 'label'; metric: string; prefix: string; start: number }
  // The caret has no suggestions.
  | { kind: 'none' }

// wordChar matches a character of a metric or label name.
const wordChar = /[a-zA-Z0-9_:]/

// suggestContext reads the word before caret in expr and whether the word
// sits inside a selector's braces.
export function suggestContext(expr: string, caret: number): SuggestContext {
  // Find the word that ends at the caret.
  let start = caret
  while (start > 0 && wordChar.test(expr[start - 1])) start--
  const prefix = expr.slice(start, caret)
  if (!prefix || /^[0-9]/.test(prefix)) return { kind: 'none' }

  // Find an unclosed brace before the word, outside quoted values.
  let open = -1
  let quoted = false
  for (let i = 0; i < start; i++) {
    const c = expr[i]
    if (c === '"' && expr[i - 1] !== '\\') quoted = !quoted
    if (quoted) continue
    if (c === '{') open = i
    if (c === '}') open = -1
  }
  if (quoted) return { kind: 'none' }
  if (open < 0) return { kind: 'metric', prefix, start }

  // Only a label name position, after { or a comma, takes a suggestion.
  const before = expr.slice(open + 1, start).trimEnd()
  if (before !== '' && !before.endsWith(',')) return { kind: 'none' }
  let m = open
  while (m > 0 && wordChar.test(expr[m - 1])) m--
  return { kind: 'label', metric: expr.slice(m, open), prefix, start }
}

// filterSuggestions returns up to limit names that start with prefix, other
// than prefix itself.
export function filterSuggestions(
  names: readonly string[],
  prefix: string,
  limit = 8,
): string[] {
  const lower = prefix.toLowerCase()
  const out: string[] = []
  for (const name of names) {
    if (name === prefix || !name.toLowerCase().startsWith(lower)) continue
    out.push(name)
    if (out.length === limit) break
  }
  return out
}
