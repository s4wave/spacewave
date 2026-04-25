import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { AuthConfirmDialog } from './AuthConfirmDialog.js'

const mockUseAccountEscalationState = vi.hoisted(() =>
  vi.fn<
    () => {
      state: { requirement?: { requiredSigners?: number } }
      loading: boolean
    }
  >(),
)
const mockAuthUnlockWizard = vi.hoisted(() => vi.fn<(props: object) => void>())

vi.mock('./useAccountEscalationState.js', () => ({
  useAccountEscalationState: () => mockUseAccountEscalationState(),
}))

vi.mock('./AuthUnlockWizard.js', () => ({
  AuthUnlockWizard: (props: {
    title: string
    description?: string
    confirmLabel?: React.ReactNode
    threshold: number
    onConfirm: () => Promise<void>
  }) => {
    mockAuthUnlockWizard(props)
    return <div>unlock-shell</div>
  },
}))

describe('AuthConfirmDialog', () => {
  it('uses the shared unlock shell for account-backed single-sig escalation', () => {
    mockUseAccountEscalationState.mockReturnValue({
      state: {
        requirement: {
          requiredSigners: 1,
        },
      },
      loading: false,
    })

    render(
      <AuthConfirmDialog
        open={true}
        onOpenChange={() => {}}
        title="Sign Out Session"
        description="Confirm your identity."
        confirmLabel="Sign Out"
        intent={{ kind: 1 }}
        onConfirm={async () => {}}
        account={
          {
            value: {},
            loading: false,
            error: null,
            retry: () => {},
          } as never
        }
      />,
    )

    expect(screen.getByText('unlock-shell')).toBeDefined()
    expect(mockAuthUnlockWizard).toHaveBeenCalledWith(
      expect.objectContaining({
        title: 'Sign Out Session',
        description: 'Confirm your identity.',
        confirmLabel: 'Sign Out',
        threshold: 0,
      }),
    )
  })
})
