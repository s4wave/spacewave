import { Run } from '@goscript/github.com/s4wave/spacewave/prototypes/opfs-browser-harness/index.js'

declare global {
  interface Window {
    __opfsResult?: { pass: boolean; detail: string }
  }
}

const result = document.querySelector<HTMLPreElement>('#result')!

try {
  const err = await Run()
  if (err != null) {
    throw err
  }
  window.__opfsResult = {
    pass: true,
    detail: 'write, close, reopen, and read-back succeeded',
  }
  result.textContent = window.__opfsResult.detail
} catch (error) {
  const detail = describeError(error)
  window.__opfsResult = { pass: false, detail }
  result.textContent = `failed: ${detail}`
}

// describeError renders any thrown value, including non-Error objects from
// generated GoScript code.
function describeError(error: unknown): string {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  if (error != null && typeof error === 'object') {
    const parts = Object.entries(error as Record<string, unknown>).map(
      ([key, value]) =>
        `${key}=${
          typeof value === 'string' || typeof value === 'number'
            ? value
            : JSON.stringify(value) ?? String(value)
        }`,
    )
    if (parts.length > 0) return `{${parts.join(', ')}}`
  }
  return String(error)
}
