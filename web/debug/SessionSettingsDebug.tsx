import { useCallback } from 'react'
import {
  LuArrowLeft,
  LuShield,
  LuCreditCard,
  LuBuilding2,
  LuKey,
  LuFingerprint,
} from 'react-icons/lu'

import { useNavigate } from '@s4wave/web/router/router.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { useStateNamespace, useStateAtom } from '@s4wave/web/state/persist.js'

// useSectionOpen returns persistent open/close state for a settings section.
function useSectionOpen(
  key: string,
  defaultOpen: boolean,
): [boolean, (open: boolean) => void] {
  const ns = useStateNamespace(['session-settings-debug'])
  return useStateAtom(ns, key, defaultOpen)
}

// SessionSettingsDebug renders CollapsibleSection variants for visual
// iteration on the settings page redesign.
export function SessionSettingsDebug() {
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  const [securityOpen, setSecurityOpen] = useSectionOpen('security', true)
  const [billingOpen, setBillingOpen] = useSectionOpen('billing', true)
  const [orgsOpen, setOrgsOpen] = useSectionOpen('orgs', false)
  const [cryptoOpen, setCryptoOpen] = useSectionOpen('crypto', false)
  const [identifiersOpen, setIdentifiersOpen] = useSectionOpen(
    'identifiers',
    false,
  )

  return (
    <div className="bg-background flex h-full w-full flex-col overflow-auto">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center gap-2 border-b px-4">
        <button
          type="button"
          onClick={goBack}
          className="text-foreground-alt hover:text-foreground transition-colors"
        >
          <LuArrowLeft className="h-4 w-4" />
        </button>
        <span className="text-foreground text-sm font-semibold tracking-tight select-none">
          Session Settings Variants
        </span>
      </div>

      <div className="flex-1 overflow-auto px-4 py-3">
        <div className="mx-auto max-w-lg space-y-3">
          <CollapsibleSection
            title="Security"
            icon={<LuShield className="h-3.5 w-3.5" />}
            open={securityOpen}
            onOpenChange={setSecurityOpen}
          >
            <div className="space-y-2">
              <PlaceholderRow label="Session Lock" detail="PIN lock enabled" />
              <PlaceholderRow
                label="Security Level"
                detail="Standard (any one method)"
              />
            </div>
          </CollapsibleSection>

          <CollapsibleSection
            title="Billing"
            icon={<LuCreditCard className="h-3.5 w-3.5" />}
            open={billingOpen}
            onOpenChange={setBillingOpen}
            badge={<StatusBadge label="Active" />}
          >
            <div className="space-y-2">
              <PlaceholderCard
                title="Personal"
                detail="Active Monthly | Next: May 12"
              />
              <PlaceholderCard
                title="Acme Corp"
                detail="Active Annual | Next: Jan 1"
              />
            </div>
          </CollapsibleSection>

          <CollapsibleSection
            title="Organizations"
            icon={<LuBuilding2 className="h-3.5 w-3.5" />}
            open={orgsOpen}
            onOpenChange={setOrgsOpen}
            badge={<CountBadge count={2} />}
          >
            <div className="space-y-2">
              <PlaceholderRow label="Acme Corp" detail="3 members" action />
              <PlaceholderRow label="Side Project" detail="1 member" action />
              <button
                type="button"
                className="text-brand/60 hover:text-brand w-full text-left text-xs transition-colors"
              >
                + Create Organization
              </button>
            </div>
          </CollapsibleSection>

          <CollapsibleSection
            title="Crypto & Keys"
            icon={<LuKey className="h-3.5 w-3.5" />}
            open={cryptoOpen}
            onOpenChange={setCryptoOpen}
          >
            <div className="space-y-2">
              <PlaceholderRow label="Key Type" detail="Ed25519" />
              <PlaceholderRow label="Export Public Key (PEM)" action />
            </div>
          </CollapsibleSection>

          <CollapsibleSection
            title="Identifiers"
            icon={<LuFingerprint className="h-3.5 w-3.5" />}
            open={identifiersOpen}
            onOpenChange={setIdentifiersOpen}
          >
            <div className="space-y-2">
              <PlaceholderRow label="Session ID" detail="abc123...def456" />
              <PlaceholderRow label="Peer ID" detail="12D3KooW...xyz" />
              <PlaceholderRow label="Account ID" detail="01HXYZ...789" />
            </div>
          </CollapsibleSection>
        </div>
      </div>
    </div>
  )
}

function PlaceholderRow({
  label,
  detail,
  action,
}: {
  label: string
  detail?: string
  action?: boolean
}) {
  return (
    <div className="flex items-center justify-between py-1">
      <span className="text-foreground text-xs">{label}</span>
      {detail && (
        <span className="text-foreground-alt/50 text-xs">{detail}</span>
      )}
      {action && !detail && (
        <span className="text-brand/60 text-xs">Export</span>
      )}
      {action && detail && (
        <span className="text-foreground-alt/30 text-xs">Open &gt;</span>
      )}
    </div>
  )
}

function PlaceholderCard({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="border-foreground/6 bg-background-card/20 rounded-md border p-2.5">
      <div className="flex items-center justify-between">
        <span className="text-foreground text-xs font-medium">{title}</span>
        <span className="text-brand/60 text-xs">Manage</span>
      </div>
      <span className="text-foreground-alt/50 text-[0.6rem]">{detail}</span>
    </div>
  )
}

function StatusBadge({ label }: { label: string }) {
  return (
    <span className="border-brand/15 text-brand/60 rounded-full border px-1.5 py-0.5 text-[0.55rem] font-medium">
      {label}
    </span>
  )
}

function CountBadge({ count }: { count: number }) {
  return <span className="text-foreground-alt/50 text-[0.55rem]">{count}</span>
}
