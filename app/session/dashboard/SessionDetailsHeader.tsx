import type { ReactNode } from 'react'
import { LuLock, LuLogOut, LuUserCog, LuX } from '@s4wave/web/images/icons.js'
import { RxPerson } from '@s4wave/web/images/icons.js'

import { CopyButton } from '@s4wave/web/ui/CopyButton.js'
import {
  DashboardButton,
  type DashboardButtonProps,
} from '@s4wave/web/ui/DashboardButton.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'
import { headerGroupMarker } from '../../../web/style/relations.stylex.js'

import {
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'
import { headerGroupMarker as headerGroupMarker2 } from '../../../web/style/relations.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    display: { default: 'none', '@media (min-width: 48rem)': 'inline' },
  },
  rule2: {
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: { minWidth: 0 },
  rule4: {
    display: 'block',
    minWidth: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule5: {
    display: 'block',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-tight)',
    '--tw-leading': 'var(--leading-tight)',
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule6: {
    display: 'flex',
    flexShrink: 0,
    flexWrap: 'wrap',
    justifyContent: 'flex-end',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule7: {
    marginLeft: themeLayout['--spacing'],
    display: { default: 'none', '@media (min-width: 48rem)': 'flex' },
    maxWidth: `calc(${themeLayout['--spacing']} * 36)`,
    minWidth: 0,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 40%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
    opacity: {
      default: 0,
      [stylex.when.ancestor(':is(:focus-within)', headerGroupMarker2)]: 1,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', headerGroupMarker2)]: 1,
      },
      '@media (pointer: coarse)': 1,
    },
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule8: { textOverflow: 'ellipsis', whiteSpace: 'nowrap', overflow: 'hidden' },
})

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: { display: { default: null, '@media (min-width: 48rem)': 'none' } },
  rule2: {
    width: {
      default: `calc(${themeLayout['--spacing']} * 5)`,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    height: {
      default: `calc(${themeLayout['--spacing']} * 5)`,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
})

// formatMiddleEllipsis shortens value to its head and tail around an ellipsis.
function formatMiddleEllipsis(value: string): string {
  const head = 6
  const tail = 8
  if (value.length <= head + tail + 1) return value
  return `${value.slice(0, head)}…${value.slice(-tail)}`
}

interface HeaderActionProps extends DashboardButtonProps {
  tooltip: string
  // collapseLabel hides the children below the md breakpoint and shows the
  // tooltip text only there.
  collapseLabel?: boolean
}

// HeaderAction renders a dashboard button with its tooltip.
function HeaderAction({
  tooltip,
  collapseLabel,
  children,
  ...props
}: HeaderActionProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <DashboardButton {...props}>
          {collapseLabel ? (
            <span sx={stylexStyles4.rule1}>{children}</span>
          ) : (
            children
          )}
        </DashboardButton>
      </TooltipTrigger>
      <TooltipContent
        side="bottom"
        sx={collapseLabel ? stylexStyles.rule1 : undefined}
      >
        {tooltip}
      </TooltipContent>
    </Tooltip>
  )
}

export interface SessionDetailsHeaderProps {
  title: string
  subtitle: string
  peerId: string
  lockDisabled: boolean
  showLogout: boolean
  loggingOut: boolean
  onChangeAccount: () => void
  onLock: () => void
  onLogout: () => void
  // onClose is omitted when the details are not dismissible.
  onClose?: () => void
}

// SessionDetailsHeader renders the session title, its peer id, and the
// account-level actions.
export function SessionDetailsHeader({
  title,
  subtitle,
  peerId,
  lockDisabled,
  showLogout,
  loggingOut,
  onChangeAccount,
  onLock,
  onLogout,
  onClose,
}: SessionDetailsHeaderProps) {
  return (
    <div sx={stylexStyles4.rule2}>
      <div sx={[stylexStyles3.rule1, headerGroupMarker]}>
        <RxPerson sx={stylexStyles2.rule1} />
        <div sx={stylexStyles4.rule3}>
          <span sx={stylexStyles4.rule4}>{title}</span>
          <span sx={stylexStyles4.rule5}>{subtitle}</span>
        </div>
        {peerId && <PeerIdBadge peerId={peerId} />}
      </div>
      <div sx={stylexStyles4.rule6}>
        <HeaderAction
          icon={<LuUserCog sx={stylexStyles2.rule2} />}
          onClick={onChangeAccount}
          tooltip="Change Account"
          collapseLabel
        >
          Change Account
        </HeaderAction>
        <HeaderAction
          icon={<LuLock sx={stylexStyles2.rule2} />}
          onClick={onLock}
          disabled={lockDisabled}
          tooltip="Lock"
          collapseLabel
        >
          Lock
        </HeaderAction>
        {showLogout && (
          <HeaderAction
            icon={<LuLogOut sx={stylexStyles2.rule2} />}
            variant="destructive"
            onClick={onLogout}
            disabled={loggingOut}
            tooltip="Logout (revoke cloud session)"
            collapseLabel={false}
          >
            <span sx={stylexStyles4.rule1}>
              {loggingOut ? 'Logging out…' : 'Logout'}
            </span>
          </HeaderAction>
        )}
        {onClose && (
          <HeaderAction
            icon={<LuX sx={stylexStyles2.rule2} />}
            onClick={onClose}
            tooltip="Close"
          />
        )}
      </div>
    </div>
  )
}

// PeerIdBadge shows the shortened peer id with a copy button on header hover,
// and always on a touch screen, which has no hover.
function PeerIdBadge({ peerId }: { peerId: string }): ReactNode {
  return (
    <span sx={stylexStyles4.rule7} title={peerId}>
      <span sx={stylexStyles4.rule8}>{formatMiddleEllipsis(peerId)}</span>
      <CopyButton
        text={peerId}
        label="Copy session ID"
        variant="toolbar"
        sx={stylexStyles.rule2}
      />
    </span>
  )
}
