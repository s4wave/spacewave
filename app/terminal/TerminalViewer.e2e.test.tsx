import { afterEach, describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

import '@s4wave/web/style/app.css'

import { TerminalFrameKind } from '@s4wave/sdk/terminal/terminal.pb.js'
import { TerminalSshTrustPanel } from './TerminalSshTrustPanel.js'

describe('TerminalSshTrustPanel browser SSH trust', () => {
  afterEach(async () => {
    await cleanup()
    vi.clearAllMocks()
  })

  it('accepts first-connect SSH host-key trust in Chromium', async () => {
    const onRespond = vi.fn<(accepted: boolean) => void>()

    await render(
      <TerminalSshTrustPanel
        challenge={{
          kind: TerminalFrameKind.SSH_HOST_KEY_TRUST_CHALLENGE,
          sshTrustHost: 'prod.internal',
          sshTrustAlgorithm: 'ssh-ed25519',
          sshTrustSha256Fingerprint: 'SHA256:abc123',
          sshTrustPublicKey: 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAID',
        }}
        onRespond={onRespond}
      />,
    )

    await expect
      .element(page.getByRole('dialog', { name: 'SSH host key trust' }))
      .toBeInTheDocument()
    await expect.element(page.getByText('prod.internal')).toBeInTheDocument()
    await expect
      .poll(() =>
        Array.from(document.querySelectorAll('span')).some(
          (el) => el.textContent === 'ssh-ed25519',
        ),
      )
      .toBe(true)
    await expect.element(page.getByText('SHA256:abc123')).toBeInTheDocument()
    await expect
      .element(page.getByText('ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAID'))
      .toBeInTheDocument()

    await page.getByRole('button', { name: /trust/i }).click()

    expect(onRespond).toHaveBeenCalledWith(true)
  })
})
