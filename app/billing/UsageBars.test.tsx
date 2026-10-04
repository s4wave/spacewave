import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { UsageBars } from './UsageBars.js'

const mockBillingState = vi.hoisted(() => ({
  selfServiceAllowed: false,
  response: {
    usage: {
      storageBytes: 80 * 2 ** 30,
      storageBaselineBytes: 100 * 2 ** 30,
      writeOps: 42_500n,
      readOps: 225_000n,
      overageLimitCents: 1000,
      monthlyPriceCents: 900,
      storageMicrodollarsPerGibMonth: 30_000,
      offerVersion: 'cloud-monthly-v3',
      policyVersion: '2026-10-02',
      accruedOverageMicrodollars: 2_000_000n,
      currentPeriodStart: 1_800_000_000_000n,
      currentPeriodEnd: 1_802_592_000_000n,
    },
  },
}))

vi.mock('./BillingStateProvider.js', () => ({
  useBillingStateContext: () => mockBillingState,
}))
vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: { useContext: () => ({ value: null }) },
}))

afterEach(cleanup)

describe('UsageBars', () => {
  it('shows accrued charges, available budget, and the subscription reset date', () => {
    render(<UsageBars />)
    expect(
      screen.getByText(/Accrued: \$2.00 · Available: \$8.00/),
    ).toBeDefined()
    expect(screen.getByText(/Subscription period:/)).toBeDefined()
    expect(screen.getByText(/Service maximum: \$19.00/)).toBeDefined()
    expect(screen.getByText(/\$0.03 per GiB-month/)).toBeDefined()
    expect(screen.getByText(/Reads and writes are not billed/)).toBeDefined()
  })

  it('shows unbilled operation counts and the included storage alert', () => {
    render(<UsageBars />)
    expect(screen.getByText('Writes 42.5K')).toBeDefined()
    expect(screen.getByText('Reads 225.0K')).toBeDefined()
    expect(
      screen.getByText(/Storage has reached 80% of included/),
    ).toBeDefined()
  })
})
