import { describe, expect, it, vi } from 'vitest'

import type { Root } from '@s4wave/sdk/root/root.js'

import { encryptForHandoffViaSession } from './handoff-state.js'

function createRoot(encryptForHandoff: () => Promise<void>) {
  const release = vi.fn()
  const mountSessionByIdx = vi.fn().mockResolvedValue({
    session: {
      release,
      spacewave: { encryptForHandoff },
    },
  })
  return {
    root: { mountSessionByIdx } as unknown as Root,
    mountSessionByIdx,
    release,
  }
}

describe('encryptForHandoffViaSession', () => {
  it('rejects invalid session indexes before mounting', async () => {
    const { root, mountSessionByIdx } = createRoot(vi.fn())

    await expect(
      encryptForHandoffViaSession(root, 0, undefined, undefined),
    ).rejects.toThrow('Invalid session index')
    expect(mountSessionByIdx).not.toHaveBeenCalled()
  })

  it('mounts, encrypts, and releases the selected session', async () => {
    const encryptForHandoff = vi.fn().mockResolvedValue(undefined)
    const { root, mountSessionByIdx, release } = createRoot(encryptForHandoff)
    const devicePublicKey = new Uint8Array([1, 2, 3])

    await encryptForHandoffViaSession(root, 4, devicePublicKey, 'nonce')

    expect(mountSessionByIdx).toHaveBeenCalledWith({ sessionIdx: 4 })
    expect(encryptForHandoff).toHaveBeenCalledWith({
      devicePublicKey,
      sessionNonce: 'nonce',
    })
    expect(release).toHaveBeenCalledOnce()
  })

  it('releases the session when encryption fails', async () => {
    const encryptForHandoff = vi.fn().mockRejectedValue(new Error('failed'))
    const { root, release } = createRoot(encryptForHandoff)

    await expect(
      encryptForHandoffViaSession(root, 4, undefined, undefined),
    ).rejects.toThrow('failed')
    expect(release).toHaveBeenCalledOnce()
  })
})
