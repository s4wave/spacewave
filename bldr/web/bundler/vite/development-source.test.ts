// @vitest-environment node
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, it } from 'vitest'

import { DevelopmentEnvironment } from './development.js'
import { DevelopmentConfig } from './vite.pb.js'

/** repoRoot is the checkout supplying the application's ordinary Vite config. */
const repoRoot = resolve(
  dirname(fileURLToPath(import.meta.url)),
  '../../../../',
)

it('attaches application and shared UI sources to one canonical graph', async () => {
  // Retain the ordinary project configuration with isolated generated inputs.
  await mkdir(join(repoRoot, '.tmp'), { recursive: true })
  const directory = await mkdtemp(join(repoRoot, '.tmp/frontend-sources-'))
  const probe = join(directory, 'Probe.tsx')
  await writeFile(
    probe,
    `export { Button } from '@s4wave/web/ui/button.js'
export { Button as SameButton } from '../../web/ui/button.js'
export { useBldrContext } from '@aptre/bldr-react'
export { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
export { createRoot } from 'react-dom/client'
`,
  )
  const environment = new DevelopmentEnvironment(
    DevelopmentConfig.create({
      rootDir: repoRoot,
      distDir: repoRoot,
      cacheDir: join(directory, 'cache'),
      entrypoints: ['app/App.tsx'],
      externalPkgs: [
        'react',
        'react-dom',
        '@aptre/bldr',
        '@aptre/bldr-react',
        '@aptre/bldr-sdk',
        '@aptre/protobuf-es-lite',
      ],
      webPkgIds: ['@s4wave/web', 'sonner'],
      sessionId: 'sources',
    }),
  )
  try {
    // Application and package aliases must reach editable modules, not snapshots.
    const result = await environment.start()
    const prefix = result.privateUrl + '/b/fe/sources/'
    const app = await fetch(prefix + 'app/App.tsx')
    expect(app.status).toBe(200)
    const appCode = await app.text()
    expect(appCode).toContain('/b/fe/sources/web/debug/DebugBridgeProvider.tsx')
    expect(appCode).toContain('/b/fe/sources/web/style/app.css')
    expect(appCode).toContain('from "react"')
    expect(appCode).toContain('from "react-dom/client"')
    expect(appCode).not.toContain('/b/pkg/@s4wave/web/')

    // Bare and relative shared imports converge while Bldr contexts stay external.
    const source = await fetch(prefix + probe.slice(repoRoot.length + 1))
    expect(source.status).toBe(200)
    const code = await source.text()
    expect(code.match(/\/b\/fe\/sources\/web\/ui\/button\.tsx/g)).toHaveLength(
      2,
    )
    expect(code).toContain('from "@aptre/bldr-react"')
    expect(code).toContain('from "@aptre/bldr-sdk/hooks/useResource.js"')
    expect(code).toContain('from "react-dom/client"')
    const button = await fetch(prefix + 'web/ui/button.tsx')
    expect(button.status).toBe(200)
    expect(await button.text()).toContain('/b/fe/sources/web/style/utils.ts')
  } finally {
    await environment.close()
    await rm(directory, { recursive: true, force: true })
  }
}, 60000)
