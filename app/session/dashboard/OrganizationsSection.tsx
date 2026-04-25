import { useCallback, useState } from 'react'
import { LuBuilding2, LuPlus } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  SessionContext,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { useStateNamespace, useStateAtom } from '@s4wave/web/state/persist.js'
import { cn } from '@s4wave/web/style/utils.js'

export interface OrganizationsSectionProps {
  open?: boolean
  onOpenChange?: (open: boolean) => void
  onNavigateToOrganization?: (orgId: string) => void
}

// OrganizationsSection shows a collapsible list of orgs with create button.
export function OrganizationsSection({
  open,
  onOpenChange,
  onNavigateToOrganization,
}: OrganizationsSectionProps) {
  const ns = useStateNamespace(['session-settings'])
  const [storedOpen, setStoredOpen] = useStateAtom(ns, 'orgs', false)
  const sectionOpen = open ?? storedOpen
  const handleOpenChange = onOpenChange ?? setStoredOpen
  const navigateSession = useSessionNavigate()

  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const { providerId } = useSessionInfo(session)
  const isLocal = providerId === 'local'

  const orgList = SpacewaveOrgListContext.useContextSafe()
  const orgs = orgList?.organizations ?? []
  const loading = orgList?.loading ?? false

  const [showCreate, setShowCreate] = useState(false)
  const [orgName, setOrgName] = useState('')
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const trimmedOrgName = orgName.trim()

  const handleCreate = useCallback(async () => {
    if (!session || !trimmedOrgName || creating) return
    setCreating(true)
    setError(null)
    try {
      await session.spacewave.createOrganization(trimmedOrgName)
      setOrgName('')
      setShowCreate(false)
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to create organization',
      )
    } finally {
      setCreating(false)
    }
  }, [creating, session, trimmedOrgName])

  const handleOpenOrganization = useCallback(
    (orgId: string) => {
      if (onNavigateToOrganization) {
        onNavigateToOrganization(orgId)
        return
      }
      navigateSession({ path: `org/${orgId}/` })
    },
    [navigateSession, onNavigateToOrganization],
  )

  if (isLocal && !orgList) return null

  const count = orgs.length

  return (
    <CollapsibleSection
      title="Organizations"
      icon={<LuBuilding2 className="h-3.5 w-3.5" />}
      open={sectionOpen}
      onOpenChange={handleOpenChange}
      badge={
        count > 0 ?
          <span className="text-foreground-alt/50 text-[0.55rem]">{count}</span>
        : undefined
      }
    >
      <div className="space-y-2">
        {loading && (
          <LoadingInline label="Loading organizations" tone="muted" size="sm" />
        )}
        {!loading && orgs.length === 0 && !showCreate && (
          <p className="text-foreground-alt text-xs">No organizations yet.</p>
        )}
        {!loading &&
          orgs.map((org) => (
            <button
              key={org.id}
              onClick={() => handleOpenOrganization(org.id ?? '')}
              className="flex w-full cursor-pointer items-center justify-between py-1 text-left"
            >
              <span className="text-foreground text-xs">
                {org.displayName || org.id}
              </span>
              <span className="text-foreground-alt/30 text-xs">Open &gt;</span>
            </button>
          ))}

        {showCreate && (
          <div className="space-y-2">
            <input
              value={orgName}
              disabled={creating}
              aria-busy={creating}
              onChange={(e) => {
                setOrgName(e.target.value)
                setError(null)
              }}
              placeholder="Organization name"
              className={cn(
                'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-1.5 text-xs transition-colors outline-none',
                'focus:border-brand/50',
              )}
              autoFocus
              onKeyDown={(e) => {
                if (e.key === 'Enter' && trimmedOrgName && !creating) {
                  void handleCreate()
                }
              }}
            />
            {creating && (
              <div className="text-foreground-alt flex items-center gap-1.5 text-[11px]">
                <Spinner size="sm" />
                <span>Creating organization...</span>
              </div>
            )}
            {error && <p className="text-destructive text-xs">{error}</p>}
            <div className="flex gap-2">
              <button
                onClick={() => void handleCreate()}
                disabled={creating || !trimmedOrgName}
                aria-busy={creating}
                className={cn(
                  'flex-1 rounded-md border py-1.5 text-xs transition-all',
                  'border-brand/30 bg-brand/10 hover:bg-brand/20',
                  'disabled:cursor-not-allowed disabled:opacity-50',
                )}
              >
                {creating ? 'Creating...' : 'Create'}
              </button>
              <button
                onClick={() => {
                  setShowCreate(false)
                  setOrgName('')
                  setError(null)
                }}
                disabled={creating}
                className="border-foreground/10 hover:bg-foreground/5 flex-1 rounded-md border py-1.5 text-xs transition-all"
              >
                Cancel
              </button>
            </div>
          </div>
        )}

        {!showCreate && (
          <button
            onClick={() => setShowCreate(true)}
            className="text-brand/60 hover:text-brand flex items-center gap-1 text-xs transition-colors"
          >
            <LuPlus className="h-3 w-3" />
            Create Organization
          </button>
        )}
      </div>
    </CollapsibleSection>
  )
}
