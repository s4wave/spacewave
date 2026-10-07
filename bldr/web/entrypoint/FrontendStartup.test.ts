import { createElement } from 'react'
import { render, waitFor, cleanup } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { FrontendStartup } from './FrontendStartup.js'

/** resolveFrontend is the document's readiness and transport attachment API. */
const resolveFrontend = vi.hoisted(() => vi.fn())
vi.mock('@aptre/bldr-react', () => ({
  useBldrContext: () => ({ webDocument: { resolveFrontend } }),
}))

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  resolveFrontend.mockReset()
  delete globalThis.__swStartupModuleImportedFrom
})

it('waits for the document attachment before importing editable startup', async () => {
  // Hold the source URL until the document's frontend resource becomes ready.
  vi.stubGlobal('__bldrFrontendStartup', 'app/startup.tsx')
  let ready!: (path: string) => void
  resolveFrontend.mockImplementation(
    () =>
      new Promise<string>((resolve) => {
        ready = resolve
      }),
  )
  render(createElement(FrontendStartup))
  expect(resolveFrontend).toHaveBeenCalledWith('app/startup.tsx')
  expect(globalThis.__swStartupModuleImportedFrom).toBeUndefined()

  // A source module may execute only after the existing attachment resolves.
  const source = new URL('./testdata/startup.ts', import.meta.url).pathname
  ready(source)
  await waitFor(() =>
    expect(globalThis.__swStartupModuleImportedFrom).toBeTruthy(),
  )
  expect(resolveFrontend).toHaveBeenCalledTimes(1)
})
