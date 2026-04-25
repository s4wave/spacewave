// Pricing constants for Spacewave cloud provider.
// Single source of truth for all pricing-related values across the app.

export const PLAN_PRICE_MONTHLY = 8
export const PLAN_PRICE_ANNUAL = 80

export const STORAGE_BASELINE_GB = 100
export const WRITE_OPS_BASELINE = 1_000_000
export const WRITE_OPS_BASELINE_DISPLAY = '1M'
export const READ_OPS_BASELINE = 10_000_000
export const READ_OPS_BASELINE_DISPLAY = '10M'

export const OVERAGE_STORAGE_PER_GB = 0.02
export const OVERAGE_WRITE_PER_MILLION = 6.0
export const OVERAGE_READ_PER_MILLION = 0.5

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
  `${STORAGE_BASELINE_GB} GB cloud storage included`,
  `${WRITE_OPS_BASELINE_DISPLAY} write operations / month`,
  `${READ_OPS_BASELINE_DISPLAY} cloud reads / month`,
  'Usage-based pricing above baseline',
]

export interface OverageItem {
  resource: string
  baseline: string
  rate: string
}

export const OVERAGE_ITEMS: OverageItem[] = [
  {
    resource: 'Storage',
    baseline: `${STORAGE_BASELINE_GB} GB`,
    rate: `$${OVERAGE_STORAGE_PER_GB.toFixed(2)} / GB-month`,
  },
  {
    resource: 'Write operations',
    baseline: `${WRITE_OPS_BASELINE_DISPLAY} / month`,
    rate: `$${OVERAGE_WRITE_PER_MILLION.toFixed(2)} / million`,
  },
  {
    resource: 'Cloud reads',
    baseline: `${READ_OPS_BASELINE_DISPLAY} / month`,
    rate: `$${OVERAGE_READ_PER_MILLION.toFixed(2)} / million`,
  },
]
