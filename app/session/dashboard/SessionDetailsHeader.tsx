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
            <span className="hidden md:inline">{children}</span>
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
    <div className="border-foreground/8 flex min-h-9 shrink-0 items-center justify-between gap-3 border-b px-4 py-2">
      <div sx={[stylexStyles3.rule1, headerGroupMarker]}>
        <RxPerson sx={stylexStyles2.rule1} />
        <div className="min-w-0">
          <span className="block min-w-0 truncate tracking-tight">{title}</span>
          <span className="text-foreground-alt/50 block truncate text-xs leading-tight font-normal">
            {subtitle}
          </span>
        </div>
        {peerId && <PeerIdBadge peerId={peerId} />}
      </div>
      <div className="flex shrink-0 flex-wrap justify-end gap-1.5">
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
            <span className="hidden md:inline">
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
    <span
      className="border-foreground/8 bg-background-card/40 text-foreground-alt/45 ml-1 hidden max-w-36 min-w-0 items-center gap-1 rounded border px-1.5 py-0.5 font-mono text-xs font-normal opacity-0 transition-opacity group-focus-within/header:opacity-100 group-hover/header:opacity-100 md:flex [@media(pointer:coarse)]:opacity-100"
      title={peerId}
    >
      <span className="truncate">{formatMiddleEllipsis(peerId)}</span>
      <CopyButton
        text={peerId}
        label="Copy session ID"
        variant="toolbar"
        sx={stylexStyles.rule2}
      />
    </span>
  )
}
