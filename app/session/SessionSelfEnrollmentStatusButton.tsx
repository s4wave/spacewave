import type { StyleXStyles as BottomBarStyles } from '@stylexjs/stylex'
import { useCallback, useState } from 'react'
import {
  LuCircleAlert,
  LuCircleCheck,
  LuKeyRound,
  LuRotateCw,
  LuSkipForward,
} from '@s4wave/web/images/icons.js'

import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from '@s4wave/web/ui/Popover.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { ProgressBar } from '@s4wave/web/ui/loading/ProgressBar.js'
import { Button } from '@s4wave/web/ui/button.js'

import {
  type SessionSelfEnrollmentStatusView,
  useSessionSelfEnrollmentStatus,
} from './SessionSelfEnrollmentStatusContext.js'

import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import * as stylex from '@stylexjs/stylex'

import { colors as themeColors } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  fonts as themeFonts,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles5 = stylex.create({
  rule1: { padding: `calc(${themeLayout['--spacing']} * 3)` },
})

const stylexStyles4 = stylex.create({
  rule1: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    color: themeColors['--color-destructive'],
  },
  rule4: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule5: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
    color: themeColors['--color-warning'],
  },
  rule6: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule7: { minWidth: 0, flex: 1 },
  rule8: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule9: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule10: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule11: {
    display: 'flex',
    flexWrap: 'wrap',
    justifyContent: 'flex-end',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule12: {
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textTransform: 'uppercase',
  },
  rule13: {
    display: 'flex',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule14: {
    minWidth: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule15: { textAlign: 'right', color: themeColors['--color-destructive'] },
})

const stylexStyles2 = stylex.create({
  rule1: { color: themeColors['--color-brand'] },
  rule2: { color: themeColors['--color-destructive'] },
  rule3: { color: themeColors['--color-warning'] },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// SessionSelfEnrollmentStatusButton registers the self-enrollment bottom-bar item.
export function SessionSelfEnrollmentStatusButton() {
  const status = useSessionSelfEnrollmentStatus()
  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, sx?: BottomBarStyles) => (
      <Popover open={selected}>
        <PopoverAnchor asChild>
          <BottomBarItem
            selected={selected}
            onClick={onClick}
            sx={[
              sx,
              !!status.running && stylexStyles2.rule1,
              !!status.failed && stylexStyles2.rule2,
              !!status.credentialRequired && stylexStyles2.rule3,
            ]}
            aria-label={status.ariaLabel}
            data-testid="session-self-enrollment-status-button"
          >
            <SessionSelfEnrollmentStatusGlyph status={status} />
          </BottomBarItem>
        </PopoverAnchor>
        <PopoverContent
          side="top"
          align="end"
          sideOffset={6}
          onEscapeKeyDown={onClick}
          onPointerDownOutside={onClick}
          variant="status"
        >
          <SessionSelfEnrollmentStatusPopover status={status} />
        </PopoverContent>
      </Popover>
    ),
    [status],
  )

  if (!status.visible) return null

  return (
    <BottomBarLevel
      id="session-self-enrollment-status"
      position="right"
      button={buttonRender}
    >
      {null}
    </BottomBarLevel>
  )
}

function SessionSelfEnrollmentStatusGlyph({
  status,
}: {
  status: SessionSelfEnrollmentStatusView
}) {
  if (status.loading || status.running) {
    return <Spinner size="sm" />
  }
  if (status.failed) {
    return <LuCircleAlert sx={stylexStyles.rule1} aria-hidden="true" />
  }
  if (status.credentialRequired) {
    return <LuKeyRound sx={stylexStyles.rule1} aria-hidden="true" />
  }
  if (status.skipped) {
    return <LuSkipForward sx={stylexStyles.rule1} aria-hidden="true" />
  }
  return <LuCircleCheck sx={stylexStyles.rule1} aria-hidden="true" />
}

// useSelfEnrollmentActions runs the start and skip actions of the status
// resource, tracking the busy flag and the last action error.
function useSelfEnrollmentActions(status: SessionSelfEnrollmentStatusView) {
  const [actionError, setActionError] = useState('')
  const [busy, setBusy] = useState(false)

  const run = useCallback(async (action: () => Promise<unknown>) => {
    setBusy(true)
    setActionError('')
    try {
      await action()
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }, [])

  const start = () => {
    const { resource } = status
    if (!resource || busy) return
    void run(() => resource.start())
  }
  const skip = () => {
    const { resource, generationKey } = status
    if (!resource || !generationKey || busy) return
    void run(() => resource.skip(generationKey))
  }

  return { actionError, busy, start, skip }
}

function SessionSelfEnrollmentStatusPopover({
  status,
}: {
  status: SessionSelfEnrollmentStatusView
}) {
  const { actionError, busy, start, skip } = useSelfEnrollmentActions(status)

  return (
    <div
      sx={stylexStyles5.rule1}
      data-testid="session-self-enrollment-status-popover"
    >
      <div sx={[nativeChildSpacingNext.s3, stylexStyles3.rule1]}>
        <div
          sx={[
            stylexStyles3.rule2,
            !!status.failed && stylexStyles3.rule3,
            !!status.running && stylexStyles3.rule4,
            !!status.credentialRequired && stylexStyles3.rule5,
            !!status.skipped && stylexStyles3.rule6,
            status.visualState === 'pending' && stylexStyles3.rule4,
          ]}
        >
          <SessionSelfEnrollmentStatusGlyph status={status} />
        </div>
        <div sx={stylexStyles3.rule7}>
          <div sx={stylexStyles3.rule8}>{status.summaryLabel}</div>
          <div sx={stylexStyles3.rule9}>{status.detailLabel}</div>
        </div>
      </div>

      {status.progressVisible && (
        <ProgressBar
          sx={nativeChildSpacingNext.s3}
          value={status.progress}
          indeterminate={status.progressIndeterminate}
        />
      )}

      <FailedSpacesList
        sx={nativeChildSpacingNext.s3}
        failures={status.failures}
      />

      {actionError && (
        <div sx={[nativeChildSpacingNext.s3, stylexStyles3.rule10]}>
          {actionError}
        </div>
      )}

      {status.startVisible && (
        <div sx={[nativeChildSpacingNext.s3, stylexStyles3.rule11]}>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!status.resource || busy || status.running}
            onClick={start}
          >
            {busy ? (
              <Spinner size="sm" />
            ) : (
              <LuRotateCw sx={stylexStyles.rule1} aria-hidden="true" />
            )}
            {status.startLabel}
          </Button>
          {status.skipVisible && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={!status.resource || busy || status.running}
              onClick={skip}
            >
              <LuSkipForward sx={stylexStyles.rule1} aria-hidden="true" />
              Skip
            </Button>
          )}
        </div>
      )}
    </div>
  )
}

// FailedSpacesList lists the first three spaces that failed to enroll.
function FailedSpacesList({
  sx,
  failures,
}: {
  failures: SessionSelfEnrollmentStatusView['failures']
} & { sx?: stylex.StyleXStyles; className?: never }) {
  if (failures.length === 0) return null

  return (
    <div sx={[stylexStyles4.rule1, sx]}>
      <div sx={[nativeChildSpacing.s1_5, stylexStyles3.rule12]}>
        Failed spaces
      </div>
      {failures.slice(0, 3).map((failure) => (
        <div
          key={failure.sharedObjectId}
          sx={[nativeChildSpacing.s1_5, stylexStyles3.rule13]}
        >
          <span sx={stylexStyles3.rule14}>{failure.sharedObjectId}</span>
          <span sx={stylexStyles3.rule15}>{failure.message}</span>
        </div>
      ))}
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
