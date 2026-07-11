import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { EntityKeypairsSection } from './EntityKeypairsSection.js'

const mocks = vi.hoisted(() => ({
  addEntityKeypair: vi.fn(),
  generateBackupKey: vi.fn(),
  downloadPemFile: vi.fn(),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => ({
      value: {
        localProvider: {
          addEntityKeypair: mocks.addEntityKeypair,
          removeEntityKeypair: vi.fn(),
        },
      },
    }),
  },
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: (resource: { value: unknown }) => resource.value,
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => ({
    value: { keypairs: [] },
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: () => ({ providerId: 'local', accountId: 'account-1' }),
}))

vi.mock('@s4wave/web/hooks/useMountAccount.js', () => ({
  useMountAccount: () => ({
    value: { generateBackupKey: mocks.generateBackupKey },
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

vi.mock('@s4wave/web/download.js', () => ({
  downloadPemFile: mocks.downloadPemFile,
}))

describe('EntityKeypairsSection', () => {
  beforeEach(() => {
    mocks.addEntityKeypair.mockResolvedValue({ peerId: 'password-peer' })
    mocks.generateBackupKey.mockResolvedValue({
      peerId: 'backup-peer',
      pemData: new Uint8Array([1, 2, 3]),
    })
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('adds only a password key from the password action', async () => {
    const user = userEvent.setup()
    render(<EntityKeypairsSection embedded />)

    await user.click(screen.getByRole('button', { name: 'Add keypair' }))
    await user.type(
      screen.getByPlaceholderText('Enter password for entity key'),
      'correct horse',
    )
    await user.click(screen.getByRole('button', { name: 'Add password key' }))

    await waitFor(() => {
      expect(mocks.addEntityKeypair).toHaveBeenCalledWith({
        credential: {
          credential: { case: 'password', value: 'correct horse' },
        },
      })
    })
    expect(mocks.generateBackupKey).not.toHaveBeenCalled()
    expect(mocks.downloadPemFile).not.toHaveBeenCalled()
  })

  it('adds and downloads only a backup key from the backup action', async () => {
    const user = userEvent.setup()
    render(<EntityKeypairsSection embedded />)

    await user.click(screen.getByRole('button', { name: 'Add keypair' }))
    await user.type(
      screen.getByPlaceholderText('Enter password for entity key'),
      'do not add this password',
    )
    await user.click(screen.getByRole('button', { name: 'Add backup key' }))

    await waitFor(() => {
      expect(mocks.generateBackupKey).toHaveBeenCalledWith({})
    })
    expect(mocks.downloadPemFile).toHaveBeenCalledWith(
      new Uint8Array([1, 2, 3]),
      'backup-key-backup-p.pem',
    )
    expect(mocks.addEntityKeypair).not.toHaveBeenCalled()
  })
})
