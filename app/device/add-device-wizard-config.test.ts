import { describe, expect, it } from 'vitest'

import {
  decodeAddDeviceWizardConfig,
  encodeAddDeviceWizardConfig,
} from './AddDeviceWizardViewer.js'

describe('Add Device wizard config storage', () => {
  it('round-trips the generated schema including explicit port zero', () => {
    const config = decodeAddDeviceWizardConfig(
      encodeAddDeviceWizardConfig({
        mode: 'ssh',
        ssh: { authMode: 'password', port: 0, setupMode: 'host' },
      }),
    )
    expect(config).toMatchObject({
      mode: 'ssh',
      ssh: { authMode: 'password', port: 0, setupMode: 'host' },
    })
  })

  it('migrates the legacy JSON storage shape at decode', () => {
    const legacy = new TextEncoder().encode(
      JSON.stringify({
        mode: 'ssh',
        ssh: { host: 'legacy.example', port: 2222, authMode: 'private-key' },
      }),
    )
    expect(decodeAddDeviceWizardConfig(legacy)).toMatchObject({
      mode: 'ssh',
      ssh: { host: 'legacy.example', port: 2222, authMode: 'private-key' },
    })
  })
})
