import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { startSSOPopupFlow } from './sso-popup.js'

class FakeBroadcastChannel {
  static channels = new Map<string, FakeBroadcastChannel>()

  public onmessage: ((event: MessageEvent) => void) | null = null

  constructor(public name: string) {
    FakeBroadcastChannel.channels.set(name, this)
  }

  close() {
    FakeBroadcastChannel.channels.delete(this.name)
  }
}

describe('sso-popup', () => {
  const openSpy = vi.fn()

  beforeEach(() => {
    openSpy.mockReset()
    vi.stubGlobal(
      'BroadcastChannel',
      FakeBroadcastChannel as unknown as typeof BroadcastChannel,
    )
    vi.stubGlobal('open', openSpy)
    vi.spyOn(crypto, 'randomUUID').mockReturnValue(
      '00000000-0000-0000-0000-000000000000',
    )
  })

  afterEach(() => {
    FakeBroadcastChannel.channels.clear()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('resolves with the auth code from the popup finish page', async () => {
    openSpy.mockReturnValue({ close: vi.fn() })

    const flow = startSSOPopupFlow({
      provider: 'github',
      ssoBaseUrl: 'https://account.test/auth/sso',
      origin: 'https://app.test',
      mode: 'unlock',
    })
    const channel = FakeBroadcastChannel.channels.get(
      'spacewave-sso:00000000-0000-0000-0000-000000000000',
    )
    channel?.onmessage?.({
      data: {
        type: 'spacewave-sso-finish',
        mode: 'unlock',
        provider: 'github',
        code: 'oauth-123',
      },
    } as MessageEvent)

    await expect(flow.waitForResult).resolves.toBe('oauth-123')
    expect(openSpy.mock.calls[0]?.[0]).toContain('mode=unlock')
  })
})
