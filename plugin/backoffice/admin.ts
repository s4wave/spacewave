import { z } from 'zod'

import type { SpacewaveSession } from '../../sdk/session/spacewave-session.js'

// Admin routes answer timestamps in Unix milliseconds, with 0 for unset.
const millis = z.number().int().nonnegative()

/** AdminAccount is one row of the coordinator's account list. */
const AdminAccountSchema = z.object({
  id: z.string(),
  entityId: z.string(),
  email: z.string().nullable(),
  emailVerifiedAt: millis.nullable(),
  subscriptionStatus: z.string(),
  lifecycleState: z.string(),
  lifecycleUpdatedAt: millis,
  deletedAt: millis,
  createdAt: millis,
  lastActiveAt: millis,
})
export type AdminAccount = z.infer<typeof AdminAccountSchema>

/** BillingSummary tallies subscribers and recurring revenue in dollars. */
const BillingSummarySchema = z.object({
  mrr: z.number(),
  arr: z.number(),
  subscribers: z.object({
    active: z.number().int(),
    past_due: z.number().int(),
    canceled: z.number().int(),
    sponsored: z.number().int(),
    total: z.number().int(),
  }),
  meter_failed_accounts: z.number().int(),
})
export type BillingSummary = z.infer<typeof BillingSummarySchema>

/** Payer is one billing account with its subscription and metered usage. */
const PayerSchema = z.object({
  id: z.string(),
  createdByAccountId: z.string(),
  displayName: z.string(),
  subscriptionStatus: z.string(),
  lifecycleState: z.string(),
  sponsored: z.boolean(),
  residentBytes: z.number().int().nonnegative(),
  accruedOverageMicrodollars: z.number().int(),
  failedMeterEvents: z.number().int(),
  pastDueSince: millis,
})
export type Payer = z.infer<typeof PayerSchema>

/** AccountResources lists the Spaces and block stores one account owns. */
const AccountResourcesSchema = z.object({
  sharedObjects: z.array(
    z.object({
      id: z.string(),
      displayName: z.string(),
      objectType: z.string(),
      ownerType: z.string(),
      publicRead: z.boolean(),
      createdAt: millis,
    }),
  ),
  blockStores: z.array(
    z.object({
      id: z.string(),
      ownerType: z.string(),
      liveBytes: z.number().int().nonnegative(),
      dmcaBlockedAt: millis,
      createdAt: millis,
    }),
  ),
})
export type AccountResources = z.infer<typeof AccountResourcesSchema>

/** Overview is the first screen of the backoffice. */
export interface Overview {
  summary: BillingSummary
  accounts: AdminAccount[]
  payers: Payer[]
}

// readAdmin reads one admin route through the session and validates its body.
async function readAdmin<T extends z.ZodType>(
  spacewave: SpacewaveSession,
  path: string,
  schema: T,
  signal: AbortSignal,
): Promise<z.infer<T>> {
  const body = await spacewave.getAdminJson(path, signal)
  return schema.parse(JSON.parse(body))
}

/** readOverview reads the billing summary, accounts matching search and payers. */
export async function readOverview(
  spacewave: SpacewaveSession,
  search: string,
  signal: AbortSignal,
): Promise<Overview> {
  const query = new URLSearchParams({ limit: '200', sort: 'last_active_at' })
  if (search) query.set('q', search)
  const [summary, accounts, payers] = await Promise.all([
    readAdmin(spacewave, 'billing/summary', BillingSummarySchema, signal),
    readAdmin(
      spacewave,
      `accounts?${query.toString()}`,
      z.object({ accounts: z.array(AdminAccountSchema) }),
      signal,
    ),
    readAdmin(
      spacewave,
      'billing/accounts?limit=200',
      z.object({ accounts: z.array(PayerSchema) }),
      signal,
    ),
  ])
  return { summary, accounts: accounts.accounts, payers: payers.accounts }
}

/** readAccountResources reads the Spaces and block stores of one account. */
export function readAccountResources(
  spacewave: SpacewaveSession,
  accountId: string,
  signal: AbortSignal,
): Promise<AccountResources> {
  return readAdmin(
    spacewave,
    `accounts/${encodeURIComponent(accountId)}/resources`,
    AccountResourcesSchema,
    signal,
  )
}

/** isAccessDenied reports whether the cloud refused the admin role check. */
export function isAccessDenied(err: Error): boolean {
  return err.message.includes('rbac_denied')
}
