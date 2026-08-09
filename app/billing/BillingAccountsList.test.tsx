import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { BillingAccountsList } from './BillingAccountsList.js'

describe('BillingAccountsList', () => {
  afterEach(cleanup)

  it('renders account identity and routes row and detach actions', () => {
    const onOpen = vi.fn()
    const onDetach = vi.fn()
    render(
      <BillingAccountsList
        accounts={[
          {
            id: 'ba_1',
            displayName: 'Production',
            assignees: [
              { ownerType: 'account', ownerId: 'acct_1', displayName: '' },
            ],
          },
        ]}
        callerAccountId="acct_1"
        assignTargets={[]}
        assigningBillingAccountId={null}
        onOpen={onOpen}
        onAssign={vi.fn()}
        onDetach={onDetach}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /Production/i }))
    expect(onOpen).toHaveBeenCalledWith('ba_1')

    fireEvent.click(screen.getByRole('button', { name: 'Detach Personal' }))
    expect(onDetach).toHaveBeenCalledWith({
      ownerType: 'account',
      ownerId: 'acct_1',
      label: 'Personal',
    })
  })
})
