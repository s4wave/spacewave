import { createElement } from 'react'
import { beforeEach, describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

import {
  DevtoolStatusAttentionSeverity,
  DevtoolStatusCommandState,
  DevtoolStatusControllerState,
  DevtoolStatusManifestState,
  DevtoolStatusPluginState,
  type DevtoolStatusSnapshot,
} from '../../devtool/status/status.pb.js'
import {
  BldrDeveloperStatusSurface,
  type DevtoolStatusViewState,
} from './BldrDeveloperStatusApp.js'

import './status.css'

function view(snapshot: DevtoolStatusSnapshot): DevtoolStatusViewState {
  return {
    connectionState: 'live',
    snapshot,
  }
}

function renderSurface(status: DevtoolStatusViewState) {
  return render(createElement(BldrDeveloperStatusSurface, { status }))
}

const runningSnapshot: DevtoolStatusSnapshot = {
  command: {
    name: 'start web',
    state: DevtoolStatusCommandState.DevtoolStatusCommandState_RUNNING,
    summary: 'serving web devtool',
    logFile: '.bldr/logs/status.log',
  },
  project: {
    projectId: 'spacewave-devtool',
    startupPlugins: ['web'],
    webStartupPath: 'bldr/web/devtool-status/startup.tsx',
    manifestIds: ['spacewave-web'],
    buildTargets: [
      {
        id: 'web',
        manifestIds: ['spacewave-web'],
        configuredTargetIds: ['browser'],
        resolvedPlatformIds: ['web/js/wasm', 'js'],
        buildTypes: ['dev', 'release'],
      },
    ],
  },
  manifestBuildRows: [
    {
      id: 'build:web',
      buildTargetIds: ['web'],
      manifestId: 'spacewave-web',
      platformId: 'web/js/wasm',
      targetPlatformIds: ['web/js/wasm', 'js'],
      buildType: 'dev',
      state: DevtoolStatusManifestState.DevtoolStatusManifestState_READY,
      cacheHit: true,
      summary: 'cache hit',
    },
  ],
  manifestFetchRows: [
    {
      id: 'fetch:web',
      manifestId: 'spacewave-web',
      platformIds: ['web/js/wasm'],
      buildTypes: ['dev'],
      remoteIds: ['devtool'],
      state: DevtoolStatusManifestState.DevtoolStatusManifestState_READY,
      readyRefCount: 1,
      summary: 'manifest ready',
    },
  ],
  controllerRows: [
    {
      id: 'controller:web',
      controllerId: 'web',
      kind: 'plugin',
      state: DevtoolStatusControllerState.DevtoolStatusControllerState_RUNNING,
    },
  ],
  pluginRows: [
    {
      id: 'plugin:web',
      pluginId: 'web',
      instanceKey: 'main',
      state: DevtoolStatusPluginState.DevtoolStatusPluginState_RUNNING,
    },
  ],
  attentionRows: [],
}

const closedSnapshot: DevtoolStatusSnapshot = {
  ...runningSnapshot,
  command: {
    name: 'start web',
    state: DevtoolStatusCommandState.DevtoolStatusCommandState_DONE,
    summary: 'done',
    logFile: '.bldr/logs/status.log',
  },
  pluginRows: [
    {
      id: 'plugin:web',
      pluginId: 'web',
      instanceKey: 'main',
      state: DevtoolStatusPluginState.DevtoolStatusPluginState_ERRORED,
      error: 'plugin failed',
    },
  ],
  attentionRows: [
    {
      id: 'attention:plugin',
      source: 'plugin',
      message: 'plugin failed',
      detail: 'startup failed',
      severity:
        DevtoolStatusAttentionSeverity.DevtoolStatusAttentionSeverity_ERROR,
    },
  ],
}

describe('BldrDeveloperStatusSurface', () => {
  beforeEach(async () => {
    await cleanup()
  })

  it('renders a typed status snapshot and updates without control actions', async () => {
    const rendered = await renderSurface(view(runningSnapshot))

    await expect.element(page.getByText('Bldr Status')).toBeInTheDocument()
    await expect
      .element(page.getByText('spacewave-devtool'))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Structured logs unavailable'))
      .toBeInTheDocument()
    expect(document.body.textContent).toContain('start web')
    expect(document.body.textContent).toContain('spacewave-web')
    expect(document.body.textContent).toContain('cache hit')
    expect(document.body.textContent).toContain('.bldr/logs/status.log')
    expect(
      document.querySelectorAll('[aria-label="Open log file"]'),
    ).toHaveLength(2)
    expect(
      document.querySelectorAll('[aria-label="Copy log path"]'),
    ).toHaveLength(2)

    await rendered.rerender(
      createElement(BldrDeveloperStatusSurface, {
        status: {
          connectionState: 'closed',
          snapshot: closedSnapshot,
        },
      }),
    )

    await expect.element(page.getByText('closed')).toBeInTheDocument()
    expect(document.body.textContent).toContain('done')
    expect(document.body.textContent).toContain('plugin failed')

    const actionText = Array.from(document.querySelectorAll('button, a'))
      .map((node) => `${node.textContent ?? ''} ${node.ariaLabel ?? ''}`)
      .join(' ')
      .toLowerCase()
    expect(actionText).not.toMatch(/restart|rebuild|refresh|reload|edit/)
  })
})
