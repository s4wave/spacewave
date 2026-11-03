import { useCallback, useMemo } from 'react'
import {
  LuArrowRight,
  LuBuilding2,
  LuChevronRight,
  LuLayers,
  LuLogIn,
  LuTriangleAlert,
} from '@s4wave/web/images/icons.js'

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

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

const stylexStyles5 = stylex.create({
  rule1: {
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-landing'],
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    height: '100%',
    width: '100%',
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 8%, transparent)`,
      },
    },
    display: 'flex',
    position: 'relative',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    zIndex: 10,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    width: '100%',
  },
  rule3: {
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    display: 'flex',
    minWidth: 0,
  },
  rule4: { minWidth: 0 },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 80%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule6: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule7: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 70%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-foreground'],
      },
    },
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule8: {
    flex: 1,
    paddingBlock: `calc(${themeLayout['--spacing']} * 8)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    position: 'relative',
    zIndex: 10,
    overflowY: 'auto',
  },
  rule9: {
    textAlign: 'center',
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule10: {
    borderRadius: 'var(--radius-xl)',
    marginInline: 'auto',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
    display: 'flex',
    justifyContent: 'center',
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule11: {
    height: `calc(${themeLayout['--spacing']} * 12)`,
    width: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule12: {
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule13: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule14: {
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
  },
  rule15: {
    fontSize: 'var(--text-base)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
  },
  rule16: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: themeLayout['--spacing'],
  },
  rule17: { maxWidth: 'var(--container-md)', width: '100%' },
  rule18: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    width: '100%',
  },
  rule19: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-normal)',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 70%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    cursor: 'pointer',
    fontSize: '10px',
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-normal)',
    lineHeight: 1.5,
    textTransform: 'none',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule20: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      [stylex.when.ancestor(':is([data-selected="true"])', groupMarker3)]:
        `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    },
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule21: { flex: 1, minWidth: 0 },
  rule22: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule23: {
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule24: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      [stylex.when.ancestor(':is([data-selected="true"])', groupMarker3)]:
        `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    },
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
})

const stylexStyles4 = stylex.create({
  rule1: { overscrollBehaviorY: 'contain' },
  rule2: { marginInline: themeLayout['--spacing'] },
})

const stylexStyles3 = stylex.create({
  rule1: {
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-destructive'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-foreground'],
      },
    },
    flexShrink: 0,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule4: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule5: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule6: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
      [stylex.when.ancestor(':is([data-selected="true"])', groupMarker)]:
        themeColors['--color-foreground-alt'],
    },
    flexShrink: 0,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
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
    <div sx={stylexStyles5.rule1}>
      {degraded && (
        <button
          type="button"
          onClick={handleIssueClick}
          sx={stylexStyles5.rule2}
        >
          <div sx={stylexStyles5.rule3}>
            <LuTriangleAlert sx={stylexStyles.rule1} />
            <div sx={stylexStyles5.rule4}>
              <p sx={stylexStyles5.rule5}>Organization root unavailable.</p>
              <p sx={stylexStyles5.rule6}>
                Spaces stay available from the session inventory while you
                review remediation options.
              </p>
            </div>
          </div>
          <div sx={[stylexStyles3.rule1, groupMarker2]}>
            <span sx={stylexStyles5.rule7}>Fix issue</span>
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

      <div sx={stylexStyles5.rule8}>
        <div sx={stylexStyles5.rule9}>
          <div
            sx={[
              stylexStyles5.rule10,
              isEmpty ? stylexStyles5.rule11 : stylexStyles5.rule12,
            ]}
          >
            <LuBuilding2
              sx={isEmpty ? stylexStyles.rule3 : stylexStyles.rule4}
            />
          </div>
          <h1
            sx={[
              stylexStyles5.rule13,
              isEmpty ? stylexStyles5.rule14 : stylexStyles5.rule15,
            ]}
          >
            {orgName}
          </h1>
          <p sx={stylexStyles5.rule16}>
            {isEmpty ? 'Create a space to get started.' : 'Organization'}
          </p>
        </div>

        <div sx={stylexStyles5.rule17}>
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
                    <span sx={stylexStyles5.rule18}>
                      <span>Spaces</span>
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation()
                          handleCreateSpace('drive')
                        }}
                        sx={stylexStyles5.rule19}
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
                      <div sx={stylexStyles5.rule20}>
                        <LuLayers sx={stylexStyles.rule5} />
                      </div>
                      <div sx={stylexStyles5.rule21}>
                        <div sx={stylexStyles5.rule22}>
                          {space.displayName || space.id}
                        </div>
                        {space.objectType && (
                          <div sx={stylexStyles5.rule23}>
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
                    <div sx={stylexStyles5.rule24}>
                      <opt.icon sx={stylexStyles2.rule1} />
                    </div>
                    <div sx={stylexStyles5.rule21}>
                      <div sx={stylexStyles5.rule22}>{opt.name}</div>
                      <div sx={stylexStyles5.rule23}>{opt.description}</div>
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
                  <div sx={stylexStyles5.rule24}>
                    <LuLogIn sx={stylexStyles.rule7} />
                  </div>
                  <div sx={stylexStyles5.rule21}>
                    <div sx={stylexStyles5.rule22}>Join Space</div>
                    <div sx={stylexStyles5.rule23}>
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
