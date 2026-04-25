import { useCallback, useMemo } from 'react'
import {
  LuArrowRight,
  LuBuilding2,
  LuChevronRight,
  LuLayers,
  LuLogIn,
  LuTriangleAlert,
} from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { useBottomBarSetOpenMenu } from '@s4wave/web/frame/bottom-bar-context.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/persist.js'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@s4wave/web/ui/command.js'
import { VISIBLE_QUICKSTART_OPTIONS } from '@s4wave/app/quickstart/options.js'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'

import { useOrgContainerState } from './OrgContainer.js'

// OrganizationDashboard renders the main content area for an organization.
// Mirrors SessionDashboard: command palette with space list and actions.
export function OrganizationDashboard() {
  const { orgId, orgName, degraded, spaces } = useOrgContainerState()
  const navigateSession = useSessionNavigate()
  const setOpenMenu = useBottomBarSetOpenMenu()
  const detailsNs = useStateNamespace(['org-details'])
  const [, setOpenSection] = useStateAtom<string | null>(
    detailsNs,
    'open-section',
    'members',
  )
  const isEmpty = spaces.length === 0

  const handleSpaceClick = useCallback(
    (spaceId: string) => {
      navigateSession({ path: `org/${orgId}/so/${spaceId}` })
    },
    [navigateSession, orgId],
  )

  const handleCreateSpace = useCallback(
    (quickstartId: string) => {
      navigateSession({ path: `org/${orgId}/new/${quickstartId}` })
    },
    [navigateSession, orgId],
  )

  const handleJoinSpace = useCallback(() => {
    navigateSession({ path: 'join' })
  }, [navigateSession])
  const handleIssueClick = useCallback(() => {
    setOpenSection('recovery')
    setOpenMenu?.('organization')
  }, [setOpenMenu, setOpenSection])

  // Filter quickstart options to space-creating ones (no account/pair/local).
  const quickstartOptions = useMemo(
    () =>
      VISIBLE_QUICKSTART_OPTIONS.filter(
        (opt) =>
          opt.id !== 'account' && opt.id !== 'pair' && opt.id !== 'local',
      ),
    [],
  )

  return (
    <div className="bg-background-landing relative flex h-full w-full flex-col overflow-hidden">
      <ShootingStars className="pointer-events-none absolute inset-0 opacity-60" />
      {degraded && (
        <button
          type="button"
          onClick={handleIssueClick}
          className="border-destructive/20 bg-destructive/5 hover:bg-destructive/8 relative z-10 flex w-full items-center border-b text-left transition-colors"
        >
          <div className="flex min-w-0 flex-1 items-start gap-2 px-3 py-1.5">
            <LuTriangleAlert className="text-destructive h-3.5 w-3.5 shrink-0" />
            <div className="min-w-0">
              <p className="text-foreground/80 text-xs font-medium">
                Organization root unavailable.
              </p>
              <p className="text-foreground-alt/60 mt-0.5 text-[11px]">
                Spaces stay available from the session inventory while you
                review remediation options.
              </p>
            </div>
          </div>
          <div className="group flex shrink-0 items-center gap-1 px-3 py-1.5 transition-colors">
            <span className="text-foreground/70 group-hover:text-foreground text-xs font-medium transition-colors">
              Fix issue
            </span>
            <LuArrowRight className="text-foreground-alt group-hover:text-foreground h-3 w-3 shrink-0 transition-colors" />
          </div>
        </button>
      )}

      <div className="relative z-10 flex flex-1 flex-col items-center justify-center overflow-y-auto px-4 py-8">
        <div className="mb-6 text-center">
          <div
            className={cn(
              'bg-brand/10 text-brand mx-auto mb-3 flex items-center justify-center rounded-xl',
              isEmpty ? 'h-12 w-12' : 'h-10 w-10',
            )}
          >
            <LuBuilding2 className={isEmpty ? 'h-6 w-6' : 'h-5 w-5'} />
          </div>
          <h1
            className={cn(
              'text-foreground font-semibold tracking-tight',
              isEmpty ? 'text-lg' : 'text-base',
            )}
          >
            {orgName}
          </h1>
          <p className="text-foreground-alt/60 mt-1 text-xs">
            {isEmpty ? 'Create a space to get started.' : 'Organization'}
          </p>
        </div>

        <div className="w-full max-w-md">
          <Command
            className={cn(
              'border-ui-outline bg-background-get-started/95 relative flex flex-col overflow-hidden rounded-lg border shadow-xl backdrop-blur-sm',
              !isEmpty ? 'h-[min(380px,60vh)]' : 'max-h-[min(380px,60vh)]',
            )}
          >
            <CommandInput
              className="border-ui-outline placeholder:text-foreground-alt/50 h-11 border-b"
              placeholder={!isEmpty ? 'Search spaces...' : 'Get started...'}
            />
            <CommandList className="min-h-0 flex-1 overflow-y-auto bg-transparent">
              <CommandEmpty className="text-foreground-alt py-8 text-center text-sm">
                No results
              </CommandEmpty>

              {!isEmpty && (
                <CommandGroup
                  heading={
                    <span className="flex w-full items-center justify-between">
                      <span>Spaces</span>
                      <button
                        onClick={(e) => {
                          e.stopPropagation()
                          handleCreateSpace('drive')
                        }}
                        className="text-brand/70 hover:text-brand cursor-pointer text-[10px] font-medium tracking-normal normal-case transition-colors"
                      >
                        + New Space
                      </button>
                    </span>
                  }
                  className="py-1"
                >
                  {spaces.map((space) => (
                    <CommandItem
                      key={space.id}
                      value={`space-${space.displayName}-${space.id}`}
                      className="group mx-1 flex cursor-pointer items-center gap-3 rounded-md px-3 py-2.5"
                      onSelect={() => handleSpaceClick(space.id ?? '')}
                    >
                      <div className="bg-brand/10 group-data-[selected=true]:bg-brand/20 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg transition-colors">
                        <LuLayers className="text-brand h-4 w-4" />
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="text-foreground truncate text-sm font-medium">
                          {space.displayName || space.id}
                        </div>
                        {space.objectType && (
                          <div className="text-foreground-alt/60 truncate text-xs">
                            {space.objectType}
                          </div>
                        )}
                      </div>
                      <LuChevronRight className="text-foreground-alt/40 group-data-[selected=true]:text-foreground-alt h-4 w-4 shrink-0 transition-colors" />
                    </CommandItem>
                  ))}
                </CommandGroup>
              )}

              <CommandGroup
                heading={isEmpty ? 'Get Started' : 'Create'}
                className="py-1"
              >
                {quickstartOptions.map((opt) => (
                  <CommandItem
                    key={opt.id}
                    value={`create-${opt.id}`}
                    className="group mx-1 flex cursor-pointer items-center gap-3 rounded-md px-3 py-2.5"
                    onSelect={() => handleCreateSpace(opt.id)}
                  >
                    <div className="bg-foreground/5 group-data-[selected=true]:bg-foreground/10 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg transition-colors">
                      <opt.icon className="text-foreground-alt h-4 w-4" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="text-foreground truncate text-sm font-medium">
                        {opt.name}
                      </div>
                      <div className="text-foreground-alt/60 truncate text-xs">
                        {opt.description}
                      </div>
                    </div>
                    <LuChevronRight className="text-foreground-alt/40 group-data-[selected=true]:text-foreground-alt h-4 w-4 shrink-0 transition-colors" />
                  </CommandItem>
                ))}
                <CommandItem
                  value="join-space"
                  className="group mx-1 flex cursor-pointer items-center gap-3 rounded-md px-3 py-2.5"
                  onSelect={handleJoinSpace}
                >
                  <div className="bg-foreground/5 group-data-[selected=true]:bg-foreground/10 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg transition-colors">
                    <LuLogIn className="text-foreground-alt h-4 w-4" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="text-foreground truncate text-sm font-medium">
                      Join Space
                    </div>
                    <div className="text-foreground-alt/60 truncate text-xs">
                      Join a shared space via invite code or link
                    </div>
                  </div>
                  <LuChevronRight className="text-foreground-alt/40 group-data-[selected=true]:text-foreground-alt h-4 w-4 shrink-0 transition-colors" />
                </CommandItem>
              </CommandGroup>
            </CommandList>
          </Command>
        </div>
      </div>
    </div>
  )
}
