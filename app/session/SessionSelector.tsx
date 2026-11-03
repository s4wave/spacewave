import { useCallback, useState, type ComponentProps } from 'react'
import {
  LuChevronDown,
  LuChevronRight,
  LuFolderOpen,
  LuFolderPlus,
  LuHardDrive,
  LuTrash2,
  LuTriangleAlert,
  LuUser,
  LuUserPlus,
} from '@s4wave/web/images/icons.js'
import { isDesktop } from '@aptre/bldr'

import { useAddSpaceRootAlias } from '@s4wave/app/hooks/useAddSpaceRootAlias.js'
import { useSpaceRootAliases } from '@s4wave/app/hooks/useSpaceRootAliases.js'
import { useSpaceRootRuntime } from '@s4wave/app/hooks/useSpaceRootRuntime.js'
import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import { useSessionAccountStatuses } from '@s4wave/app/hooks/useSessionAccountStatuses.js'
import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'
import { useSelectAccount } from '@s4wave/app/hooks/useSelectAccount.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { Button } from '@s4wave/web/ui/button.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'
import type { SessionListEntry } from '@s4wave/core/session/session.pb.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import {
  SpaceRootRuntimeStatus,
  SpaceRootStatus,
  type SpaceRootRuntimeSession,
  type SpaceRootAliasRecord,
  type WatchSpaceRootRuntimeResponse,
} from '@s4wave/sdk/root/root.pb.js'

import { accountDescription, accountTitle } from './account-presentation.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    overflowX: 'hidden',
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background-landing'],
  },
  rule2: {
    position: 'relative',
    zIndex: 10,
    display: 'flex',
    minHeight: '100%',
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule3: {
    fontSize: 'var(--text-2xl)',
    lineHeight: 'var(--tw-leading, var(--text-2xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
  },
  rule4: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule5: {
    position: 'relative',
    zIndex: 10,
    paddingBottom: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'center',
  },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule7: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule8: { width: '100%', maxWidth: 'var(--container-sm)' },
  rule9: { width: '100%', maxWidth: 'var(--container-md)' },
  rule10: {
    marginTop: `calc(${themeLayout['--spacing']} * 6)`,
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule11: {
    marginTop: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule12: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule13: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
  },
  rule14: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule15: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
        ':disabled': { default: null, ':hover': '#0000' },
      },
    },
    cursor: { default: null, ':disabled': 'default' },
  },
  rule16: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 9)`,
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule17: { minWidth: 0, flex: 1 },
  rule18: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule19: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule20: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule21: { color: themeColors['--color-warning'] },
  rule22: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-warning'],
  },
  rule23: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule24: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule25: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule26: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule27: {
    display: 'flex',
    minWidth: 0,
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule28: { flexShrink: 0 },
  rule29: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule30: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule31: {
    display: 'inline-flex',
    maxWidth: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
  },
  rule32: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
  },
  rule33: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 12)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-warning'],
  },
  rule34: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule35: { opacity: 0.5 },
  rule36: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 9)`,
    height: `calc(${themeLayout['--spacing']} * 9)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule37: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule38: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule39: {
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '9px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    color: themeColors['--color-brand'],
    textTransform: 'uppercase',
  },
  rule40: {
    borderRadius: '3.40282e38px',
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
  },
  rule41: {
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 75%, transparent)`,
  },
  rule42: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-warning'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule4: { rotate: '-90deg' },
  rule5: {
    marginTop: '1px',
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
  rule7: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
})

// SessionSelector displays a full-page session picker for users with multiple sessions.
export function SessionSelector() {
  const resource = useSessionList()
  const rootAliases = useSpaceRootAliases()
  const addRootAlias = useAddSpaceRootAlias()
  const accountStatuses = useSessionAccountStatuses()
  const navigate = useNavigate()
  const sessions = resource.value?.sessions ?? []
  const rootRecords = rootAliases.value?.records ?? []
  // undefined means the user has not chosen; null means every root is closed.
  const [chosenRootAliasId, setChosenRootAliasId] = useState<
    string | null | undefined
  >(undefined)
  // Without local sessions, the first ready root opens by default.
  const selectedRootAliasId =
    chosenRootAliasId !== undefined
      ? chosenRootAliasId
      : sessions.length === 0
        ? (rootRecords.find(isRootAliasReady)?.aliasId ?? null)
        : null
  const selectedRootRuntime = useSpaceRootRuntime(selectedRootAliasId)

  const handleAddAccount = useCallback(() => {
    navigate({ path: '/login' })
  }, [navigate])

  const handleHome = useCallback(() => {
    navigate({ path: '/landing' })
  }, [navigate])

  if (resource.loading || (sessions.length === 0 && rootAliases.loading)) {
    return (
      <SessionSelectorStatus
        view={{
          state: 'loading',
          title: 'Loading sessions',
          detail: 'Reading available sessions from the provider.',
        }}
      />
    )
  }

  if (resource.error) {
    return (
      <SessionSelectorStatus
        view={{
          state: 'error',
          title: 'Failed to load sessions',
          error: resource.error.message,
          onRetry: resource.retry,
        }}
      />
    )
  }

  if (sessions.length === 0 && rootRecords.length === 0) {
    return <NavigatePath to="/landing" replace />
  }

  return (
    <div sx={stylexStyles2.rule1} data-testid="session-selector">
      <BackButton floating onClick={handleHome}>
        Home
      </BackButton>

      <div sx={stylexStyles2.rule2}>
        <AnimatedLogo followMouse={true} containerClassName="mb-6" />

        <h1 sx={stylexStyles2.rule3}>Welcome back</h1>
        <p sx={stylexStyles2.rule4}>
          {sessions.length > 0
            ? 'Choose a session to continue'
            : 'Accounts found in your state roots'}
        </p>

        {sessions.length > 0 && (
          <div
            className={
              /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'w-full max-w-md space-y-2'
            }
          >
            {sessions.map((session) => (
              <SessionCard
                key={session.sessionIndex}
                session={session}
                accountStatus={accountStatuses.get(session.sessionIndex ?? 0)}
              />
            ))}
          </div>
        )}

        <StateRootList
          records={rootRecords}
          showHeading={sessions.length > 0}
          selectedAliasId={selectedRootAliasId}
          selectedRuntime={selectedRootRuntime.value}
          onOpenChange={setChosenRootAliasId}
        />

        <SelectorActions
          addRootAlias={addRootAlias}
          onAddAccount={handleAddAccount}
          onRootAdded={setChosenRootAliasId}
        />
      </div>

      <div sx={stylexStyles2.rule5}>
        <p sx={stylexStyles2.rule6}>local-first · encrypted</p>
      </div>
    </div>
  )
}

// SessionSelectorStatus renders a centered loading or error card.
function SessionSelectorStatus(props: {
  view: ComponentProps<typeof LoadingCard>['view']
}) {
  return (
    <div sx={stylexStyles2.rule7}>
      <div sx={stylexStyles2.rule8}>
        <LoadingCard view={props.view} />
      </div>
    </div>
  )
}

// StateRootList renders the configured state roots, opening the selected one.
function StateRootList(props: {
  records: SpaceRootAliasRecord[]
  showHeading: boolean
  selectedAliasId: string | null
  selectedRuntime?: WatchSpaceRootRuntimeResponse | null
  onOpenChange: (aliasId: string | null) => void
}) {
  const { records, selectedAliasId } = props
  if (records.length === 0) return null

  return (
    <div sx={stylexStyles2.rule9}>
      {props.showHeading && <h2 sx={stylexStyles2.rule10}>State roots</h2>}
      <div
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
        }
      >
        {records.map((record) => (
          <SpaceRootCard
            key={record.aliasId}
            record={record}
            open={record.aliasId === selectedAliasId}
            runtime={
              record.aliasId === selectedAliasId ? props.selectedRuntime : null
            }
            onOpenChange={props.onOpenChange}
          />
        ))}
      </div>
    </div>
  )
}

// SelectorActions renders the add-account and add-state-root controls.
// onRootAdded receives the alias of a newly added state root.
function SelectorActions(props: {
  addRootAlias: ReturnType<typeof useAddSpaceRootAlias>
  onAddAccount: () => void
  onRootAdded: (aliasId: string) => void
}) {
  const { addRootAlias, onRootAdded } = props

  return (
    <>
      <div sx={stylexStyles2.rule11}>
        <Button variant="outline" onClick={props.onAddAccount}>
          <LuUserPlus sx={stylexStyles.rule1} />
          Add account
        </Button>
        <Button
          variant="outline"
          onClick={() => {
            void addRootAlias.add().then((aliasId) => {
              if (aliasId) onRootAdded(aliasId)
            })
          }}
          disabled={!addRootAlias.canAdd}
        >
          <LuFolderPlus sx={stylexStyles.rule1} />
          {addRootAlias.adding ? 'Adding state root' : 'Add state root'}
        </Button>
      </div>
      {!isDesktop && (
        <p sx={stylexStyles2.rule12}>
          State roots can be added in the desktop app.
        </p>
      )}
    </>
  )
}

// SpaceRootCard renders a configured state root and, when open, its accounts.
function SpaceRootCard(props: {
  record: SpaceRootAliasRecord
  open: boolean
  runtime?: WatchSpaceRootRuntimeResponse | null
  onOpenChange: (aliasId: string | null) => void
}) {
  const { record, open, runtime, onOpenChange } = props
  const rootResource = useRootResource()
  const root = rootResource.value
  const [removing, setRemoving] = useState(false)
  const ready = isRootAliasReady(record)
  const aliasId = record.aliasId ?? ''

  const handleRemove = useCallback(async () => {
    if (!root || !aliasId || removing) return
    setRemoving(true)
    try {
      await root.removeSpaceRootAlias(aliasId)
      if (open) onOpenChange(null)
    } catch (err) {
      toast.error('Could not remove state root', { description: String(err) })
    } finally {
      setRemoving(false)
    }
  }, [aliasId, onOpenChange, open, removing, root])

  const handleToggle = useCallback(() => {
    if (!ready || !aliasId) return
    onOpenChange(open ? null : aliasId)
  }, [aliasId, onOpenChange, open, ready])

  return (
    <div sx={stylexStyles2.rule13} data-testid="space-root-card">
      <div sx={stylexStyles2.rule14}>
        <button
          type="button"
          onClick={handleToggle}
          disabled={!ready}
          aria-expanded={open}
          sx={stylexStyles2.rule15}
        >
          <div sx={stylexStyles2.rule16}>
            {ready ? (
              <LuHardDrive sx={stylexStyles.rule1} />
            ) : (
              <LuTriangleAlert sx={stylexStyles.rule2} />
            )}
          </div>
          <div sx={stylexStyles2.rule17}>
            <div sx={stylexStyles2.rule18}>{record.displayName || aliasId}</div>
            <div
              sx={[
                stylexStyles2.rule19,
                ready ? stylexStyles2.rule20 : stylexStyles2.rule21,
              ]}
            >
              {record.statusMessage || record.native?.path}
            </div>
          </div>
          {ready && (
            <LuChevronDown
              sx={[stylexStyles.rule3, !open && stylexStyles.rule4]}
            />
          )}
        </button>
        <Button
          onClick={() => {
            void handleRemove()
          }}
          disabled={!root || removing}
          aria-label="Remove state root"
          variant="muted"
          size="iconSm"
        >
          <LuTrash2 sx={stylexStyles.rule1} />
        </Button>
      </div>
      {open && <SpaceRootAccounts runtime={runtime} />}
    </div>
  )
}

// isRootAliasReady reports whether a configured root can be opened.
function isRootAliasReady(record: SpaceRootAliasRecord): boolean {
  return (
    record.status === SpaceRootStatus.SpaceRootStatus_READY ||
    record.status === SpaceRootStatus.SpaceRootStatus_UNKNOWN
  )
}

// SpaceRootAccounts renders the accounts served by an open state root.
function SpaceRootAccounts(props: {
  runtime?: WatchSpaceRootRuntimeResponse | null
}) {
  const runtime = props.runtime
  const status = runtime?.status

  if (status === SpaceRootRuntimeStatus.SpaceRootRuntimeStatus_ERROR) {
    return (
      <div sx={stylexStyles2.rule22}>
        <LuTriangleAlert sx={stylexStyles.rule5} />
        <span>{runtime?.error || 'State root unavailable'}</span>
      </div>
    )
  }
  if (status !== SpaceRootRuntimeStatus.SpaceRootRuntimeStatus_READY) {
    return (
      <div sx={stylexStyles2.rule23}>
        <Spinner size="sm" />
        {status === SpaceRootRuntimeStatus.SpaceRootRuntimeStatus_STARTING
          ? 'Starting the state root daemon'
          : 'Connecting to the state root'}
      </div>
    )
  }

  const runtimeSessions = runtime?.runtimeSessions?.length
    ? runtime.runtimeSessions
    : (runtime?.sessions ?? []).map((session): SpaceRootRuntimeSession => ({
        session,
      }))
  if (runtimeSessions.length === 0) {
    return <div sx={stylexStyles2.rule24}>No accounts in this state root.</div>
  }

  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: divide-y */ 'border-foreground/10 divide-foreground/10 divide-y border-t'
      }
    >
      {runtimeSessions.map((runtimeSession) => (
        <SpaceRootAccount
          key={runtimeSession.session?.sessionIndex}
          runtimeSession={runtimeSession}
        />
      ))}
    </div>
  )
}

// SpaceRootAccount renders one account and its spaces in a state root.
function SpaceRootAccount(props: { runtimeSession: SpaceRootRuntimeSession }) {
  const { runtimeSession } = props
  const meta = runtimeSession.metadata
  const provider =
    meta?.providerDisplayName ||
    (meta?.providerId === 'spacewave' ? 'Cloud' : 'Local')
  const title =
    meta?.displayName || meta?.cloudEntityId || `${provider} account`
  const accountId = meta?.providerAccountId
  const spaces = runtimeSession.spaces ?? []

  return (
    <div sx={stylexStyles2.rule25}>
      <div sx={stylexStyles2.rule26}>
        <div sx={stylexStyles2.rule16}>
          <LuUser sx={stylexStyles.rule1} />
        </div>
        <div sx={stylexStyles2.rule17}>
          <div sx={stylexStyles2.rule18}>{title}</div>
          <div sx={stylexStyles2.rule27}>
            <span sx={stylexStyles2.rule28}>
              {spaces.length === 1 ? '1 space' : `${spaces.length} spaces`}
            </span>
            {accountId && (
              <span sx={stylexStyles2.rule29} title={accountId}>
                · {accountId}
              </span>
            )}
          </div>
        </div>
      </div>
      {spaces.length > 0 && (
        <div sx={stylexStyles2.rule30}>
          {spaces.map((space) => (
            <span
              key={
                space.entry?.ref?.providerResourceRef?.id ??
                [space.entry?.source, space.spaceMeta?.name].join(':')
              }
              sx={stylexStyles2.rule31}
            >
              <LuFolderOpen sx={stylexStyles.rule6} />
              <span sx={stylexStyles2.rule32}>
                {space.spaceMeta?.name || 'Untitled space'}
              </span>
            </span>
          ))}
        </div>
      )}
      {runtimeSession.error && (
        <div sx={stylexStyles2.rule33}>{runtimeSession.error}</div>
      )}
    </div>
  )
}

// SessionCard renders a single session entry in the selector list.
function SessionCard(props: {
  session: SessionListEntry
  accountStatus?: ProviderAccountStatus
}) {
  const selectAccount = useSelectAccount()
  const meta = useSessionMetadata(props.session.sessionIndex ?? null)
  const isCloudProvider = meta?.providerId === 'spacewave'
  const isLinked = meta?.providerId === 'local' && !!meta?.cloudAccountId
  const isInactive =
    props.accountStatus === ProviderAccountStatus.ProviderAccountStatus_DORMANT
  const sessionIndex = props.session.sessionIndex ?? 0
  const title = accountTitle(meta, sessionIndex)
  const subtitle = accountDescription(meta, sessionIndex)

  const handleSessionSelect = useCallback(() => {
    if (sessionIndex) selectAccount(sessionIndex)
  }, [selectAccount, sessionIndex])

  const handleSessionSelectKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      if (e.key !== 'Enter' && e.key !== ' ') return
      e.preventDefault()
      handleSessionSelect()
    },
    [handleSessionSelect],
  )

  return (
    <div
      role="button"
      tabIndex={0}
      data-testid="session-card"
      data-session-index={props.session.sessionIndex ?? ''}
      onClick={handleSessionSelect}
      onKeyDown={handleSessionSelectKeyDown}
      sx={[stylexStyles2.rule34, !!isLinked && stylexStyles2.rule35]}
    >
      <div sx={stylexStyles2.rule36}>
        <LuUser sx={stylexStyles.rule1} />
      </div>
      <div sx={stylexStyles2.rule17}>
        <div sx={stylexStyles2.rule37}>
          <span sx={stylexStyles2.rule38}>{title}</span>
          {isCloudProvider && <span sx={stylexStyles2.rule39}>Cloud</span>}
          {isLinked && <span sx={stylexStyles2.rule40}>(linked)</span>}
          {isInactive && <span sx={stylexStyles2.rule41}>(Inactive)</span>}
        </div>
        <div sx={stylexStyles2.rule42}>{subtitle}</div>
      </div>
      <LuChevronRight sx={stylexStyles.rule7} />
    </div>
  )
}
