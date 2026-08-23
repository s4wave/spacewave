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
    throw new Error(String(err))
  }
  window.__opfsResult = {
    pass: true,
    detail: 'write, close, reopen, and read-back succeeded',
  }
  result.textContent = window.__opfsResult.detail
} catch (error) {
  const detail = error instanceof Error ? error.message : String(error)
  window.__opfsResult = { pass: false, detail }
  result.textContent = `failed: ${detail}`
}
