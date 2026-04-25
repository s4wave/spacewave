// localStorage helpers for session isolation tests.
// Supports setItem and getItem operations via a single compiled script.
export default function (args: {
  op: 'set' | 'get'
  key: string
  value?: string
}): string | null {
  if (args.op === 'set') {
    localStorage.setItem(args.key, args.value ?? '')
    return null
  }
  return localStorage.getItem(args.key)
}
