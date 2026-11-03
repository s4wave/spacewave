import { useCallback, useMemo } from 'react'
import {
  LuArrowRight,
  LuBuilding2,
  LuChevronRight,
  LuLayers,
  LuLogIn,
  LuTriangleAlert,
} from '@s4wave/web/images/icons.js'

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
import { useVisibleQuickstartOptions } from '@s4wave/app/quickstart/useQuickstartOptions.js'
import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'

import { MissingBillingAccountBanner } from './MissingBillingAccountBanner.js'
import { useOrgContainerState } from './OrgContainer.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: { overscrollBehaviorY: 'contain' },
  rule2: { marginInline: themeLayout['--spacing'] },
})

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-destructive'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    flexShrink: 0,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-brand'],
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
      [stylex.when.ancestor(':is([data-selected="true"])', groupMarker)]:
        themeColors['--color-foreground-alt'],
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule7: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-foreground-alt'],
  },
})

// OrganizationDashboard renders the main content area for an organization.
// Mirrors SessionDashboard: command palette with space list and actions.
export function OrganizationDashboard() {
  // Read the organization snapshot and its persisted details-menu selection.
  const {
    orgId,
    orgName,
    orgState,
    degraded,
    isOwner,
    spaces,
    billingAccountId,
  } = useOrgContainerState()
  const navigateSession = useSessionNavigate()
  const setOpenMenu = useBottomBarSetOpenMenu()
  const detailsNs = useStateNamespace(['org-details'])
  const [, setOpenSection] = useStateAtom<string | null>(
    detailsNs,
    'open-section',
    'members',
  )
  const isEmpty = spaces.length === 0

  // Open the selected Space inside the current organization.
  const handleSpaceClick = useCallback(
    (spaceId: string) => {
      navigateSession({ path: `org/${orgId}/so/${spaceId}` })
    },
    [navigateSession, orgId],
  )

  // Open a new Space using the selected quickstart.
  const handleCreateSpace = useCallback(
    (quickstartId: string) => {
      navigateSession({ path: `org/${orgId}/new/${quickstartId}` })
    },
    [navigateSession, orgId],
  )

  // Connect join, recovery and billing actions to their existing routes.
  const handleJoinSpace = useCallback(() => {
    navigateSession({ path: 'join' })
  }, [navigateSession])
  const handleIssueClick = useCallback(() => {
    setOpenSection('recovery')
    setOpenMenu?.('organization')
  }, [setOpenMenu, setOpenSection])
  const handleConfigureBilling = useCallback(() => {
    navigateSession({ path: 'billing' })
  }, [navigateSession])

  // Project the billing banner and permitted quickstart options.
  const showMissingBillingBanner = !!orgState && !billingAccountId
  const visibleQuickstartOptions = useVisibleQuickstartOptions()

  // Filter quickstart options to space-creating ones (no account/pair/local).
  const quickstartOptions = useMemo(
    () =>
      visibleQuickstartOptions.filter(
        (opt) =>
          opt.id !== 'account' && opt.id !== 'pair' && opt.id !== 'local',
      ),
    [visibleQuickstartOptions],
  )

  return (
    <div className="bg-background-landing relative flex h-full w-full flex-col overflow-hidden">
      {degraded && (
        <button
          type="button"
          onClick={handleIssueClick}
          className="border-destructive/20 bg-destructive/5 hover:bg-destructive/8 relative z-10 flex w-full items-center border-b text-left transition-colors"
        >
          <div className="flex min-w-0 flex-1 items-start gap-2 px-3 py-1.5">
            <LuTriangleAlert sx={stylexStyles.rule1} />
            <div className="min-w-0">
              <p className="text-foreground/80 text-xs font-medium">
                Organization root unavailable.
              </p>
              <p className="text-foreground-alt/60 mt-0.5 text-xs">
                Spaces stay available from the session inventory while you
                review remediation options.
              </p>
            </div>
          </div>
          <div sx={[stylexStyles3.rule1, groupMarker2]}>
            <span className="text-foreground/70 group-hover:text-foreground text-xs font-medium transition-colors">
              Fix issue
            </span>
            <LuArrowRight sx={stylexStyles.rule2} />
          </div>
        </button>
      )}

      {showMissingBillingBanner && (
        <MissingBillingAccountBanner
          isOwner={isOwner}
          onConfigureBilling={handleConfigureBilling}
        />
      )}

      <div className="relative z-10 flex flex-1 flex-col items-center justify-center overflow-y-auto px-4 py-8">
        <div className="mb-6 text-center">
          <div
            className={cn(
              'bg-brand/10 text-brand mx-auto mb-3 flex items-center justify-center rounded-xl',
              isEmpty ? 'size-12' : 'size-10',
            )}
          >
            <LuBuilding2
              sx={isEmpty ? stylexStyles.rule3 : stylexStyles.rule4}
            />
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
          <Command variant="dashboard">
            <CommandInput
              variant="dashboard"
              placeholder={!isEmpty ? 'Search spaces...' : 'Get started...'}
            />
            <CommandList variant="transparent" sx={stylexStyles4.rule1}>
              <CommandEmpty variant="dashboard">No results</CommandEmpty>

              {!isEmpty && (
                <CommandGroup
                  heading={
                    <span className="flex w-full items-center justify-between">
                      <span>Spaces</span>
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation()
                          handleCreateSpace('drive')
                        }}
                        className="text-brand/70 hover:text-brand micro-ten cursor-pointer font-medium tracking-normal normal-case transition-colors"
                      >
                        + New Space
                      </button>
                    </span>
                  }
                  variant="compact"
                >
                  {spaces.map((space) => (
                    <CommandItem
                      key={space.id}
                      value={`space-${space.displayName}-${space.id}`}
                      variant="dashboard"
                      sx={stylexStyles4.rule2}
                      onSelect={() => handleSpaceClick(space.id ?? '')}
                    >
                      <div className="bg-brand/10 group-data-[selected=true]:bg-brand/20 flex size-7 shrink-0 items-center justify-center rounded-md transition-colors">
                        <LuLayers sx={stylexStyles.rule5} />
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
                      <LuChevronRight sx={stylexStyles.rule6} />
                    </CommandItem>
                  ))}
                </CommandGroup>
              )}

              <CommandGroup
                heading={isEmpty ? 'Get Started' : 'Create'}
                variant="compact"
              >
                {quickstartOptions.map((opt) => (
                  <CommandItem
                    key={opt.id}
                    value={`create-${opt.id}`}
                    variant="dashboard"
                    sx={stylexStyles4.rule2}
                    onSelect={() => handleCreateSpace(opt.id)}
                  >
                    <div className="bg-foreground/5 group-data-[selected=true]:bg-foreground/10 flex size-7 shrink-0 items-center justify-center rounded-md transition-colors">
                      <opt.icon sx={stylexStyles2.rule1} />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="text-foreground truncate text-sm font-medium">
                        {opt.name}
                      </div>
                      <div className="text-foreground-alt/60 truncate text-xs">
                        {opt.description}
                      </div>
                    </div>
                    <LuChevronRight sx={stylexStyles.rule6} />
                  </CommandItem>
                ))}
                <CommandItem
                  value="join-space"
                  variant="dashboard"
                  sx={stylexStyles4.rule2}
                  onSelect={handleJoinSpace}
                >
                  <div className="bg-foreground/5 group-data-[selected=true]:bg-foreground/10 flex size-7 shrink-0 items-center justify-center rounded-md transition-colors">
                    <LuLogIn sx={stylexStyles.rule7} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="text-foreground truncate text-sm font-medium">
                      Join Space
                    </div>
                    <div className="text-foreground-alt/60 truncate text-xs">
                      Join a shared space via invite code or link
                    </div>
                  </div>
                  <LuChevronRight sx={stylexStyles.rule6} />
                </CommandItem>
              </CommandGroup>
            </CommandList>
          </Command>
        </div>
      </div>
    </div>
  )
}
