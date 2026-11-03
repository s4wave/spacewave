import {
  LuArrowRight,
  LuArrowRightLeft,
  LuIdCard,
  LuLock,
  LuLockOpen,
  LuUser,
} from '@s4wave/web/images/icons.js'

import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import {
  SessionLockMode,
  type SessionListEntry,
} from '@s4wave/core/session/session.pb.js'
import type { SpaceSoListEntry } from '@s4wave/core/space/space.pb.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { useBottomBarSetOpenMenu } from '@s4wave/web/frame/bottom-bar-context.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

import { Facts } from './Facts.js'
import { InspectorSection } from './InspectorSection.js'
import { spaceEngineId } from './useSystemModel.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import {
  radii as themeRadii,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

import { utilityState as themeUtilityState } from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 48rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule3: { minWidth: 0, flex: 1 },
  rule4: {
    display: 'block',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule5: {
    display: 'block',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule6: {
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule7: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule8: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
  },
  rule9: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule10: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule11: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule12: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule13: { color: themeColors['--color-brand'] },
  rule14: { marginTop: `calc(${themeLayout['--spacing']} * 2)` },
  rule15: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule16: {
    marginRight: 'auto',
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 3%, transparent)`,
      },
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
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
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// SpacesInspector lists every account session on this device and every Space
// in the current session, with actions to switch to or open each one.
export function SpacesInspector({
  sessions,
  spaces,
  onClose,
}: {
  sessions: SessionListEntry[] | null
  spaces: SpaceSoListEntry[] | null
  onClose: () => void
}) {
  const navigate = useNavigate()
  const sessionIndex = useSessionIndex()

  return (
    <div sx={stylexStyles3.rule1}>
      <InspectorSection
        title="Accounts"
        description="Every session on this device. Each one keeps its own keys and Spaces."
      >
        {!sessions ? (
          <p sx={stylexStyles3.rule2}>Reading sessions…</p>
        ) : (
          <ul
            className={
              /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
            }
          >
            {sessions.map((session) => (
              <SessionRow
                key={session.sessionIndex}
                sessionIndex={session.sessionIndex ?? 0}
                current={session.sessionIndex === sessionIndex}
                onClose={onClose}
              />
            ))}
          </ul>
        )}
      </InspectorSection>

      <InspectorSection
        title="Spaces in this session"
        description="Spaces mounted by the current session."
      >
        {!spaces ? (
          <p sx={stylexStyles3.rule2}>Reading Spaces…</p>
        ) : spaces.length === 0 ? (
          <p sx={stylexStyles3.rule2}>This session has no Spaces yet.</p>
        ) : (
          <ul
            className={
              /* TODO(stylex): a descendant, group, peer or compound selector: divide-y */ 'divide-foreground/6 -mx-2 divide-y'
            }
          >
            {spaces.map((space) => {
              const id = space.entry?.ref?.providerResourceRef?.id ?? ''
              const name = space.spaceMeta?.name || 'Untitled Space'
              return (
                <li key={spaceEngineId(space)}>
                  <button
                    type="button"
                    disabled={!id}
                    onClick={() => {
                      navigate({ path: `/u/${sessionIndex}/so/${id}` })
                      onClose()
                    }}
                    sx={[stylexStyles2.rule1, groupMarker2]}
                    aria-label={`Open ${name}`}
                  >
                    <span sx={stylexStyles3.rule3}>
                      <span sx={stylexStyles3.rule4}>{name}</span>
                      <span sx={stylexStyles3.rule5}>{id || 'unknown'}</span>
                    </span>
                    <span sx={stylexStyles3.rule6}>
                      {space.entry?.source === 'shared' ? 'Shared' : 'Owned'}
                    </span>
                    <LuArrowRight sx={stylexStyles.rule1} aria-hidden="true" />
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </InspectorSection>
    </div>
  )
}

// SessionRow is one account session with its provider facts and actions.
function SessionRow({
  sessionIndex,
  current,
  onClose,
}: {
  sessionIndex: number
  current: boolean
  onClose: () => void
}) {
  const metadata = useSessionMetadata(sessionIndex)
  const navigate = useNavigate()
  const setOpenMenu = useBottomBarSetOpenMenu()
  const name = metadata?.displayName || `Session ${sessionIndex}`
  const autoUnlock = metadata?.lockMode === SessionLockMode.AUTO_UNLOCK

  return (
    <li
      sx={[
        stylexStyles3.rule7,
        current ? stylexStyles3.rule8 : stylexStyles3.rule9,
      ]}
    >
      <div sx={stylexStyles3.rule10}>
        <LuUser sx={stylexStyles.rule2} aria-hidden="true" />
        <span sx={stylexStyles3.rule11}>{name}</span>
        <span sx={stylexStyles3.rule12}>/u/{sessionIndex}</span>
        {current && <span sx={stylexStyles3.rule13}>Current</span>}
      </div>

      <div sx={stylexStyles3.rule14}>
        <Facts
          facts={[
            { label: 'Provider', value: metadata?.providerDisplayName },
            {
              label: 'Account',
              value: metadata?.cloudAccountId || metadata?.providerAccountId,
              mono: true,
            },
            { label: 'Entity', value: metadata?.cloudEntityId, mono: true },
            {
              label: 'Created',
              value: metadata?.createdAt
                ? new Date(Number(metadata.createdAt)).toLocaleDateString(
                    'en-US',
                    { month: 'short', day: 'numeric', year: 'numeric' },
                  )
                : null,
            },
          ]}
        />
      </div>

      <div sx={stylexStyles3.rule15}>
        {metadata?.lockMode != null && (
          <span sx={stylexStyles3.rule16}>
            {autoUnlock ? (
              <LuLockOpen sx={stylexStyles.rule3} aria-hidden="true" />
            ) : (
              <LuLock sx={stylexStyles.rule3} aria-hidden="true" />
            )}
            {autoUnlock ? 'Unlocks automatically' : 'Locked with a PIN'}
          </span>
        )}
        {!current && (
          <DashboardButton
            icon={<LuArrowRightLeft sx={stylexStyles.rule3} />}
            onClick={() => {
              navigate({ path: `/u/${sessionIndex}` })
              onClose()
            }}
          >
            Switch to account
          </DashboardButton>
        )}
        <DashboardButton
          icon={<LuIdCard sx={stylexStyles.rule3} />}
          onClick={() => {
            if (!current) {
              navigate({ path: `/u/${sessionIndex}` })
            }
            queueMicrotask(() => setOpenMenu?.('account'))
          }}
        >
          Account details
        </DashboardButton>
      </div>
    </li>
  )
}
