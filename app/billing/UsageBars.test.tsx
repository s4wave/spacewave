import type { ReactNode } from 'react'

import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { UsageBars } from './UsageBars.js'

const mockBillingState = vi.hoisted(() => ({
  response: {
    usage: {
      storageBytes: 112.4 * 1024 * 1024 * 1024,
      storageBaselineBytes: 100 * 1024 * 1024 * 1024,
      writeOps: 1n,
      writeOpsBaseline: 10n,
      readOps: 1n,
      readOpsBaseline: 10n,
      storageOverageBytes: 12.4 * 1024 * 1024 * 1024,
      storageOverageMonthlyCostEstimateUsd: 0.25,
      storageOverageMonthToDateGbMonths: 0.5,
      storageOverageMonthToDateCostEstimateUsd: 0.01,
      storageOverageDeletedGbMonths: 0,
      storageOverageDeletedCostEstimateUsd: 0,
      usageMeteredThroughAt: 1776900000000n,
    },
  },
}))

vi.mock('./BillingStateProvider.js', () => ({
  useBillingStateContext: () => mockBillingState,
}))

vi.mock('@s4wave/web/ui/tooltip.js', () => ({
  Tooltip: ({ children }: { children?: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children?: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children?: ReactNode }) => <>{children}</>,
}))

describe('UsageBars', () => {
  beforeEach(() => {
    mockBillingState.response.usage.storageBytes = 112.4 * 1024 * 1024 * 1024
    mockBillingState.response.usage.storageBaselineBytes =
      100 * 1024 * 1024 * 1024
    mockBillingState.response.usage.storageOverageBytes =
      12.4 * 1024 * 1024 * 1024
    mockBillingState.response.usage.storageOverageMonthlyCostEstimateUsd = 0.25
    mockBillingState.response.usage.storageOverageMonthToDateGbMonths = 0.5
    mockBillingState.response.usage.storageOverageMonthToDateCostEstimateUsd = 0.01
    mockBillingState.response.usage.storageOverageDeletedGbMonths = 0
    mockBillingState.response.usage.storageOverageDeletedCostEstimateUsd = 0
    mockBillingState.response.usage.usageMeteredThroughAt = 1776900000000n
  })

  afterEach(() => {
    cleanup()
  })

  it('shows storage overage as an extra cost line', () => {
    render(<UsageBars />)

    expect(screen.getByText('Extra storage')).toBeDefined()
    expect(screen.getByText('12.4 GB')).toBeDefined()
    expect(screen.getByText('$0.02')).toBeDefined()
    expect(screen.getByText('$0.25/mo')).toBeDefined()
    expect(screen.getByText('Month-to-date overage')).toBeDefined()
    expect(screen.getByText('0.500 GB-months = $0.01 estimated')).toBeDefined()
    expect(screen.getByText(/Accrued storage overage/)).toBeDefined()
    expect(
      screen.getByText('Usage metered through 2026-04-22 23:20 UTC'),
    ).toBeDefined()
  })

  it('does not show storage overage below the included baseline', () => {
    mockBillingState.response.usage.storageBytes = 50 * 1024 * 1024 * 1024
    mockBillingState.response.usage.storageOverageBytes = 0

    render(<UsageBars />)

    expect(screen.queryByText('Extra storage')).toBeNull()
  })

  it('shows already-deleted data cost only when provided', () => {
    mockBillingState.response.usage.storageOverageDeletedGbMonths = 0.25
    mockBillingState.response.usage.storageOverageDeletedCostEstimateUsd = 0.01

    render(<UsageBars />)

    expect(screen.getByText('Already-deleted data')).toBeDefined()
    expect(screen.getByText('0.250 GB-months = +$0.01 estimated')).toBeDefined()
    expect(screen.getByText(/data that is no longer stored/)).toBeDefined()
  })
})
