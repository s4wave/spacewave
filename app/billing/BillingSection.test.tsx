import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render } from '@testing-library/react'

import { BillingSection } from './BillingSection.js'

const mockBillingStateContextSafe = vi.hoisted(() => vi.fn())
const mockNavigateSession = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  useSessionNavigate: () => mockNavigateSession,
}))

vi.mock('@s4wave/web/ui/CollapsibleSection.js', () => ({
  CollapsibleSection: ({ children }: { children?: ReactNode }) => (
    <div>{children}</div>
  ),
}))

vi.mock('@s4wave/web/state/persist.js', () => ({
  useStateNamespace: () => ['session-settings'],
  useStateAtom: () => [true, vi.fn()],
}))

vi.mock('@s4wave/web/contexts/SpacewaveOrgListContext.js', () => ({
  SpacewaveOrgListContext: {
    useContextSafe: () => null,
  },
}))

vi.mock('./BillingStateProvider.js', () => ({
  useBillingStateContextSafe: mockBillingStateContextSafe,
}))

vi.mock('./BillingAccountCard.js', () => ({
  BillingAccountCard: () => null,
}))

describe('BillingSection', () => {
  afterEach(() => {
    cleanup()
    mockBillingStateContextSafe.mockReset()
  })

  it('returns null for local sessions without requiring billing context', () => {
    mockBillingStateContextSafe.mockReturnValue(null)

    const view = render(<BillingSection isLocal={true} />)

    expect(view.container.firstChild).toBeNull()
    expect(mockBillingStateContextSafe).toHaveBeenCalledTimes(1)
  })
})
