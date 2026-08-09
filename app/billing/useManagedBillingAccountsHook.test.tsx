import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { useManagedBillingAccounts } from './useManagedBillingAccounts.js'

const current = vi.hoisted(() => ({ session: null as unknown }))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: { useContext: () => ({ value: current.session }) },
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: () => current.session,
}))

function Consumer() {
  const { data, loading } = useManagedBillingAccounts()
  return <div>{loading ? 'loading' : data?.accounts?.[0]?.id || 'empty'}</div>
}

describe('useManagedBillingAccounts', () => {
  afterEach(() => {
    cleanup()
    current.session = null
  })

  it('switches snapshots when the selected session changes', async () => {
    current.session = {
      spacewave: {
        listManagedBillingAccounts: vi
          .fn()
          .mockResolvedValue({ accounts: [{ id: 'ba_first' }] }),
      },
    }
    const { rerender } = render(<Consumer />)
    await waitFor(() => expect(screen.getByText('ba_first')).toBeDefined())

    current.session = {
      spacewave: {
        listManagedBillingAccounts: vi
          .fn()
          .mockResolvedValue({ accounts: [{ id: 'ba_second' }] }),
      },
    }
    rerender(<Consumer />)

    await waitFor(() => expect(screen.getByText('ba_second')).toBeDefined())
  })
})
