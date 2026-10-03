import cloudOffer from '@s4wave/core/provider/spacewave/api/cloud-offer.json'

export const CLOUD_OFFER = cloudOffer
export const PLAN_PRICE_MONTHLY = CLOUD_OFFER.monthlyPriceCents / 100
export const STORAGE_BASELINE_GB = CLOUD_OFFER.storageBytes / 2 ** 30

// formatStorageRate renders an offer's extra storage price per GiB-month.
export function formatStorageRate(microdollarsPerGibMonth: number): string {
  return `$${(microdollarsPerGibMonth / 1_000_000).toFixed(2)} per GiB-month`
}

// formatSpendingLimits lists the nonzero spending limits an offer allows.
export function formatSpendingLimits(limitsCents: number[]): string {
  const limits = limitsCents.filter((cents) => cents > 0)
  const names = limits.map((cents) => `$${cents / 100}`)
  return `${names.slice(0, -1).join(', ')}, or ${names[names.length - 1]}`
}

export const STORAGE_RATE_DISPLAY = formatStorageRate(
  CLOUD_OFFER.storageMicrodollarsPerGibMonth,
)
export const DEFAULT_LIMIT_DISPLAY = `$${CLOUD_OFFER.defaultOverageLimitCents / 100}`
export const OVERAGE_EXPLANATION = `The plan includes ${STORAGE_BASELINE_GB} GiB of storage. Storage above it costs ${STORAGE_RATE_DISPLAY}, measured hourly, up to the monthly spending limit you choose: ${formatSpendingLimits(CLOUD_OFFER.overageLimitsCents)}, starting at ${DEFAULT_LIMIT_DISPLAY}. You can also turn extra storage off. Reads and writes are not billed; rate limits protect the service from abuse. At the limit, uploads pause until the period renews or you raise it.`

export const FREE_FEATURES = [
  'Full local-first app on your devices',
  'No account, sign-up, or payment required',
  'Stores data on your devices',
  'Peer-to-peer sync directly between devices',
  'Full plugin SDK and developer tools',
  'End-to-end encrypted by default',
  'Open-source, self-hostable',
]

export const CLOUD_FEATURES = [
  'Adds cloud sync, storage, backup, and relay services:',
  'Cloud sync and backup across all devices',
  'Shared Spaces with collaborators',
  `${STORAGE_BASELINE_GB} GiB cloud storage included`,
  'Unmetered reads and writes, with fair-use rate limits',
  `Extra storage at ${STORAGE_RATE_DISPLAY} up to a limit you control`,
]

export interface OverageItem {
  resource: string
  baseline: string
  rate: string
}

export const OVERAGE_ITEMS: OverageItem[] = [
  {
    resource: 'Storage',
    baseline: `${STORAGE_BASELINE_GB} GiB`,
    rate: STORAGE_RATE_DISPLAY,
  },
  {
    resource: 'Writes',
    baseline: 'Rate limited',
    rate: 'Not billed',
  },
  {
    resource: 'Reads',
    baseline: 'Rate limited',
    rate: 'Not billed',
  },
]
