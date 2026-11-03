import { useState, type ReactNode } from 'react'
import {
  LuCloud,
  LuGitBranch,
  LuKeyboard,
  LuLogOut,
  LuMerge,
  LuTerminal,
  LuTrash2,
} from '@s4wave/web/images/icons.js'

import type { Session } from '@s4wave/sdk/session/session.js'
import { useInvokeCommand } from '@s4wave/web/command/index.js'
import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'
import { useNavigate } from '@s4wave/web/router/router.js'

import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'

import { LogoutConfirmDialog } from '../LogoutConfirmDialog.js'
import { DeleteAccountDialog } from './DeleteAccountDialog.js'
import { DeleteSpaceEscapeHatchDialog } from './DeleteSpaceEscapeHatchDialog.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker } from '../../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: { display: 'flex', minWidth: 0, flex: 1, flexDirection: 'column' },
  rule2: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule3: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      },
    },
  },
  rule2: {
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      },
    },
  },
  rule3: { color: themeColors['--color-foreground'] },
  rule4: { color: themeColors['--color-foreground-alt'] },
  rule5: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      },
    },
  },
  rule6: {
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      },
    },
  },
  rule7: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-warning']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-warning'],
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-warning']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
      },
    },
  },
  rule8: {
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          `color-mix(in oklab, ${themeColors['--color-warning']} 30%, transparent)`,
      },
    },
  },
  rule9: { color: themeColors['--color-warning'] },
  rule10: {
    color: `color-mix(in oklab, ${themeColors['--color-warning']} 80%, transparent)`,
  },
  rule11: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
  },
  rule12: {
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
      },
    },
  },
  rule13: {
    color: themeColors['--color-destructive'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule14: {
    color: `color-mix(in oklab, ${themeColors['--color-destructive']} 80%, transparent)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule15: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive-foreground'],
      },
    },
  },
  rule16: {
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          `color-mix(in oklab, ${themeColors['--color-destructive-foreground']} 20%, transparent)`,
      },
    },
  },
  rule17: {
    color: {
      default: themeColors['--color-destructive'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-destructive-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule18: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 80%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          `color-mix(in oklab, ${themeColors['--color-destructive-foreground']} 80%, transparent)`,
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-foreground-alt'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-brand'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-warning'],
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-destructive'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: {
      default: themeColors['--color-destructive'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-destructive-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule6: {
    display: 'flex',
    width: '100%',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule7: { cursor: 'not-allowed', opacity: 0.5 },
  rule8: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 7)`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule9: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule10: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
})

// ActionTone holds the compiled styles that give an action card its tone.
interface ActionTone {
  card: import('@stylexjs/stylex').StyleXStyles
  iconBox: import('@stylexjs/stylex').StyleXStyles
  icon: import('@stylexjs/stylex').StyleXStyles
  title: import('@stylexjs/stylex').StyleXStyles
  description: import('@stylexjs/stylex').StyleXStyles
}

const neutralTone: ActionTone = {
  card: stylexStyles2.rule1,
  iconBox: stylexStyles2.rule2,
  icon: stylexStyles.rule1,
  title: stylexStyles2.rule3,
  description: stylexStyles2.rule4,
}

const brandTone: ActionTone = {
  card: stylexStyles2.rule5,
  iconBox: stylexStyles2.rule6,
  icon: stylexStyles.rule2,
  title: stylexStyles2.rule3,
  description: stylexStyles2.rule4,
}

const warningTone: ActionTone = {
  card: stylexStyles2.rule7,
  iconBox: stylexStyles2.rule8,
  icon: stylexStyles.rule3,
  title: stylexStyles2.rule9,
  description: stylexStyles2.rule10,
}

const destructiveTone: ActionTone = {
  card: stylexStyles2.rule11,
  iconBox: stylexStyles2.rule12,
  icon: stylexStyles.rule4,
  title: stylexStyles2.rule13,
  description: stylexStyles2.rule14,
}

const destructiveStrongTone: ActionTone = {
  card: stylexStyles2.rule15,
  iconBox: stylexStyles2.rule16,
  icon: stylexStyles.rule5,
  title: stylexStyles2.rule17,
  description: stylexStyles2.rule18,
}

interface ActionCardProps {
  tone: ActionTone
  icon: ReactNode
  title: ReactNode
  description: ReactNode
  onClick: () => void
  disabled?: boolean
}

// ActionCard renders a full-width action with an icon, title, and description.
function ActionCard({
  sx,
  tone,
  icon,
  title,
  description,
  onClick,
  disabled,
}: ActionCardProps & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      sx={[
        [
          [stylexStyles.rule6, groupMarker],
          tone.card,
          disabled && stylexStyles.rule7,
        ],
        sx,
      ]}
    >
      <div sx={[stylexStyles.rule8, tone.iconBox]}>{icon}</div>
      <div sx={stylexStyles3.rule1}>
        <h4 sx={[stylexStyles.rule9, tone.title]}>{title}</h4>
        <p sx={[stylexStyles.rule10, tone.description]}>{description}</p>
      </div>
    </button>
  )
}

export interface SessionActionsSectionProps {
  session: Session | null | undefined
  sessionIdx: number | null
  isLocal: boolean
  showTransfer: boolean
  showLogout: boolean
  loggingOut: boolean
  logoutOpen: boolean
  dangerZoneOpen: boolean
  onDangerZoneOpenChange: (open: boolean) => void
  onLogoutOpenChange: (open: boolean) => void
  onLogoutClick: () => void
  onLogoutConfirm: () => void
  onUpgradeToCloud: () => void
  // onDeleteAccountRedirect closes the details before a cloud account is
  // deleted through its own flow.
  onDeleteAccountRedirect: () => void
}

// SessionActionsSection renders the account actions and their confirmation
// dialogs.
export function SessionActionsSection({
  sx,
  session,
  sessionIdx,
  isLocal,
  showTransfer,
  showLogout,
  loggingOut,
  logoutOpen,
  dangerZoneOpen,
  onDangerZoneOpenChange,
  onLogoutOpenChange,
  onLogoutClick,
  onLogoutConfirm,
  onUpgradeToCloud,
  onDeleteAccountRedirect,
}: SessionActionsSectionProps & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const navigate = useNavigate()
  const navigateSession = useSessionNavigate()
  const invokeCommand = useInvokeCommand()
  const [deleteAcctOpen, setDeleteAcctOpen] = useState(false)
  const [deleteSpaceOpen, setDeleteSpaceOpen] = useState(false)

  const handleDeleteAccount = async () => {
    if (!isLocal && sessionIdx != null) {
      setDeleteAcctOpen(false)
      onDeleteAccountRedirect()
      navigateSession({ path: 'delete-account' })
      return
    }
    if (session && sessionIdx != null) {
      await session.deleteAccount(sessionIdx)
    }
    navigate({ path: '/sessions' })
  }

  return (
    <section sx={sx}>
      <div sx={stylexStyles3.rule2}>
        <h2 sx={stylexStyles3.rule3}>Actions</h2>
      </div>

      <div>
        {isLocal && sessionIdx != null && (
          <ActionCard
            sx={nativeChildSpacing.s2}
            tone={brandTone}
            icon={<LuCloud sx={brandTone.icon} />}
            title="Upgrade to Cloud"
            description="Sync across devices with Spacewave Cloud"
            onClick={onUpgradeToCloud}
          />
        )}

        <ActionCard
          sx={nativeChildSpacing.s2}
          tone={neutralTone}
          icon={<LuKeyboard sx={neutralTone.icon} />}
          title="Keyboard Shortcuts"
          description="Open account keyboard shortcut overrides"
          onClick={() =>
            invokeCommand('spacewave.preferences.keyboard-shortcuts', {
              scope: 'account',
            })
          }
        />

        <ActionCard
          sx={nativeChildSpacing.s2}
          tone={neutralTone}
          icon={<LuTerminal sx={neutralTone.icon} />}
          title="Command Line"
          description="Connect the spacewave CLI to this session"
          onClick={() => navigateSession({ path: 'settings/cli' })}
        />

        <ActionCard
          sx={nativeChildSpacing.s2}
          tone={neutralTone}
          icon={<LuGitBranch sx={neutralTone.icon} />}
          title="Plugins"
          description="Add a plugin from GitHub and check it for updates"
          onClick={() => navigateSession({ path: 'settings/plugins' })}
        />

        {showTransfer && (
          <ActionCard
            sx={nativeChildSpacing.s2}
            tone={neutralTone}
            icon={<LuMerge sx={neutralTone.icon} />}
            title="Transfer Sessions"
            description="Merge spaces from another session into this one"
            onClick={() => navigateSession({ path: 'settings/transfer' })}
          />
        )}

        {showLogout && (
          <ActionCard
            sx={nativeChildSpacing.s2}
            tone={warningTone}
            icon={<LuLogOut sx={warningTone.icon} />}
            title={loggingOut ? 'Logging out…' : 'Log Out'}
            description="Sign out and remove session data"
            onClick={onLogoutClick}
            disabled={loggingOut}
          />
        )}

        <CollapsibleSection
          sx={nativeChildSpacing.s2}
          title="Danger Zone"
          open={dangerZoneOpen}
          onOpenChange={onDangerZoneOpenChange}
        >
          <div>
            <ActionCard
              sx={nativeChildSpacing.s2}
              tone={destructiveTone}
              icon={<LuTrash2 sx={destructiveTone.icon} />}
              title="Delete a Space"
              description="Permanently remove a broken space without opening it"
              onClick={() => setDeleteSpaceOpen(true)}
              disabled={!session}
            />
            <ActionCard
              sx={nativeChildSpacing.s2}
              tone={destructiveStrongTone}
              icon={<LuTrash2 sx={destructiveStrongTone.icon} />}
              title={isLocal ? 'Delete Local Data' : 'Delete Account'}
              description={
                isLocal
                  ? 'Permanently remove this account and all local data'
                  : 'Permanently delete this account and all data'
              }
              onClick={() => setDeleteAcctOpen(true)}
            />
          </div>
        </CollapsibleSection>
      </div>

      <DeleteAccountDialog
        open={deleteAcctOpen}
        onOpenChange={setDeleteAcctOpen}
        isCloud={!isLocal}
        onConfirm={handleDeleteAccount}
      />
      <DeleteSpaceEscapeHatchDialog
        open={deleteSpaceOpen}
        onOpenChange={setDeleteSpaceOpen}
        session={session ?? null}
      />
      <LogoutConfirmDialog
        open={logoutOpen}
        onOpenChange={onLogoutOpenChange}
        loggingOut={loggingOut}
        onConfirm={onLogoutConfirm}
      />
    </section>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
