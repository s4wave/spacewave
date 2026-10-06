import { useCallback, useState, type FormEvent, type ReactNode } from 'react'
import { LuRefreshCw, LuSearch, LuShieldOff } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { Input } from '@s4wave/web/ui/input.js'
import type { SpacewaveSession } from '../../sdk/session/spacewave-session.js'
import {
  isAccessDenied,
  readAccountResources,
  readOverview,
  type AdminAccount,
  type BillingSummary,
  type Payer,
} from './admin.js'

/**
 * BackofficeViewer reads accounts, billing and storage from the cloud through
 * the viewer's own Session. The cloud answers only accounts that hold the
 * platform admin role. Data loads when the viewer opens and on Refresh.
 */
export default function BackofficeViewer(_props: ObjectViewerComponentProps) {
  const session = useResourceValue(SessionContext.useContext())
  const spacewave = session?.spacewave
  const [draft, setDraft] = useState('')
  const [search, setSearch] = useState('')
  const [reloadKey, setReloadKey] = useState(0)
  const [selected, setSelected] = useState<AdminAccount | null>(null)
  const overview = usePromise(
    useCallback(
      (signal: AbortSignal) =>
        spacewave ? readOverview(spacewave, search, signal) : undefined,
      [spacewave, search, reloadKey], // eslint-disable-line react-hooks/exhaustive-deps -- reloadKey rereads on Refresh
    ),
  )
  const refresh = useCallback(() => setReloadKey((key) => key + 1), [])
  const submitSearch = useCallback(
    (event: FormEvent) => {
      event.preventDefault()
      setSearch(draft.trim())
    },
    [draft],
  )

  // A refused role check shows the access message and no data.
  if (overview.error) {
    if (isAccessDenied(overview.error)) {
      return (
        <EmptyState
          icon={<LuShieldOff />}
          title="No access to the backoffice"
          description="This account does not hold the Spacewave admin role. Sign in with an admin account to read accounts and billing."
        />
      )
    }
    return (
      <ErrorState
        title="Could not read the backoffice"
        message={overview.error.message}
        onRetry={refresh}
      />
    )
  }
  if (!overview.data) {
    return (
      <p role="status" className="p-4">
        Reading accounts and billing…
      </p>
    )
  }
  const { summary, accounts, payers } = overview.data

  return (
    <section className="flex h-full flex-col gap-6 overflow-auto p-4">
      <header className="flex items-center justify-between gap-4">
        <h2 className="text-xl font-semibold">Spacewave backoffice</h2>
        <Button
          variant="outline"
          size="sm"
          onClick={refresh}
          disabled={overview.loading}
        >
          <LuRefreshCw />
          Refresh
        </Button>
      </header>

      <SummaryPanel summary={summary} />

      <Panel title={`Accounts (${accounts.length})`}>
        <form onSubmit={submitSearch} className="flex gap-2">
          <Input
            aria-label="Search accounts"
            placeholder="Email or account ID"
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
          />
          <Button type="submit" variant="outline" size="sm">
            <LuSearch />
            Search
          </Button>
        </form>
        <Table
          head={['Account', 'Subscription', 'State', 'Created', 'Last active']}
        >
          {accounts.map((account) => (
            <tr
              key={account.id}
              className={cn(selected?.id === account.id && 'bg-brand/10')}
            >
              <td>
                <button
                  type="button"
                  aria-pressed={selected?.id === account.id}
                  onClick={() => setSelected(account)}
                  className="text-left hover:underline"
                >
                  <div>{account.email ?? 'No email'}</div>
                  <div className="text-foreground-alt font-mono text-xs">
                    {account.id}
                  </div>
                </button>
              </td>
              <td>{account.subscriptionStatus}</td>
              <td>{account.deletedAt ? 'deleted' : account.lifecycleState}</td>
              <td>{formatDate(account.createdAt)}</td>
              <td>{formatDate(account.lastActiveAt)}</td>
            </tr>
          ))}
        </Table>
      </Panel>

      {selected && spacewave && (
        <ResourcesPanel spacewave={spacewave} account={selected} />
      )}

      <Panel title={`Payers (${payers.length})`}>
        <PayerTable payers={payers} />
      </Panel>
    </section>
  )
}

// SummaryPanel shows subscriber counts and recurring revenue.
function SummaryPanel({ summary }: { summary: BillingSummary }) {
  const { subscribers } = summary
  const figures: [string, string][] = [
    ['Active', String(subscribers.active)],
    ['Past due', String(subscribers.past_due)],
    ['Canceled', String(subscribers.canceled)],
    ['Sponsored', String(subscribers.sponsored)],
    ['MRR', formatDollars(summary.mrr)],
    ['ARR', formatDollars(summary.arr)],
    ['Meter failures', String(summary.meter_failed_accounts)],
  ]
  return (
    <Panel title="Billing">
      <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-7">
        {figures.map(([label, value]) => (
          <div key={label} className="border-foreground/10 rounded border p-3">
            <dt className="text-foreground-alt text-xs">{label}</dt>
            <dd className="text-lg font-semibold">{value}</dd>
          </div>
        ))}
      </dl>
    </Panel>
  )
}

// ResourcesPanel reads and shows the Spaces and block stores of one account.
function ResourcesPanel({
  spacewave,
  account,
}: {
  spacewave: SpacewaveSession
  account: AdminAccount
}) {
  const resources = usePromise(
    useCallback(
      (signal: AbortSignal) =>
        readAccountResources(spacewave, account.id, signal),
      [spacewave, account.id],
    ),
  )
  const title = `Resources of ${account.email ?? account.id}`

  if (resources.error) {
    return (
      <Panel title={title}>
        <p role="alert">{resources.error.message}</p>
      </Panel>
    )
  }
  if (!resources.data) {
    return (
      <Panel title={title}>
        <p role="status">Reading resources…</p>
      </Panel>
    )
  }
  const { sharedObjects, blockStores } = resources.data

  return (
    <Panel title={title}>
      <Table head={['Space', 'Type', 'Owner', 'Public', 'Created']}>
        {sharedObjects.map((so) => (
          <tr key={so.id}>
            <td>
              <div>{so.displayName || 'Untitled'}</div>
              <div className="text-foreground-alt font-mono text-xs">
                {so.id}
              </div>
            </td>
            <td>{so.objectType}</td>
            <td>{so.ownerType}</td>
            <td>{so.publicRead ? 'yes' : 'no'}</td>
            <td>{formatDate(so.createdAt)}</td>
          </tr>
        ))}
      </Table>
      <Table head={['Block store', 'Owner', 'Live bytes', 'DMCA', 'Created']}>
        {blockStores.map((bs) => (
          <tr key={bs.id}>
            <td className="font-mono text-xs">{bs.id}</td>
            <td>{bs.ownerType}</td>
            <td>{formatBytes(bs.liveBytes)}</td>
            <td>{bs.dmcaBlockedAt ? formatDate(bs.dmcaBlockedAt) : 'no'}</td>
            <td>{formatDate(bs.createdAt)}</td>
          </tr>
        ))}
      </Table>
    </Panel>
  )
}

// PayerTable shows billing accounts with their subscription and usage.
function PayerTable({ payers }: { payers: Payer[] }) {
  return (
    <Table
      head={[
        'Payer',
        'Subscription',
        'State',
        'Stored',
        'Accrued overage',
        'Past due since',
      ]}
    >
      {payers.map((payer) => (
        <tr key={payer.id}>
          <td>
            <div>
              {payer.displayName || payer.createdByAccountId}
              {payer.sponsored && ' (sponsored)'}
            </div>
            <div className="text-foreground-alt font-mono text-xs">
              {payer.id}
            </div>
          </td>
          <td>{payer.subscriptionStatus}</td>
          <td>{payer.lifecycleState}</td>
          <td>{formatBytes(payer.residentBytes)}</td>
          <td>{formatDollars(payer.accruedOverageMicrodollars / 1e6)}</td>
          <td>{formatDate(payer.pastDueSince)}</td>
        </tr>
      ))}
    </Table>
  )
}

// Panel titles one section of the backoffice.
function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3">
      <h3 className="font-semibold">{title}</h3>
      {children}
    </section>
  )
}

// Table renders rows under a header with the backoffice's cell spacing.
function Table({ head, children }: { head: string[]; children: ReactNode }) {
  return (
    <table className="w-full text-left text-sm [&_td]:px-2 [&_td]:py-1.5 [&_th]:px-2 [&_th]:py-1.5">
      <thead className="text-foreground-alt text-xs">
        <tr>
          {head.map((label) => (
            <th key={label} className="font-medium">
              {label}
            </th>
          ))}
        </tr>
      </thead>
      <tbody className="divide-foreground/5 divide-y">{children}</tbody>
    </table>
  )
}

// formatDate renders Unix milliseconds as a local date, or "never" for 0.
function formatDate(ms: number): string {
  return ms ? new Date(ms).toLocaleDateString() : 'never'
}

// formatDollars renders an amount in US dollars.
function formatDollars(amount: number): string {
  return amount.toLocaleString(undefined, {
    style: 'currency',
    currency: 'USD',
  })
}

// formatBytes renders a byte count in binary units.
function formatBytes(bytes: number): string {
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${value.toFixed(unit ? 1 : 0)} ${units[unit]}`
}
