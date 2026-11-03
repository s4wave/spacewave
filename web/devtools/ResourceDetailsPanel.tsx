import { useCallback, useEffect, useMemo, useState } from 'react'
import { LuCopy, LuRefreshCw, LuX } from '@s4wave/web/images/icons.js'

import {
  useResourceDevToolsContext,
  type TrackedResource,
} from '@aptre/bldr-sdk/hooks/ResourceDevToolsContext.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
  motion as themeMotion,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule2: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    fontSize: themeLayout['--text-devtools'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-devtools--line-height']})`,
  },
  rule2: {
    color: themeColors['--color-text-secondary'],
    fontSize: themeLayout['--text-ui'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-text-secondary'],
    fontSize: themeLayout['--text-ui'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
  },
  rule2: {
    display: 'flex',
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '.25rem',
    backgroundColor: {
      default: themeColors['--color-background-deep'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-pulldown-hover'],
      },
    },
    paddingBlock: themeLayout['--spacing'],
    color: {
      default: themeColors['--color-text-secondary'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.1s',
    '--tw-duration': '.1s',
    fontSize: themeLayout['--text-ui'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 56)`,
    flexShrink: 0,
    flexDirection: 'column',
    overflow: 'hidden',
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: '1px',
    borderColor: themeColors['--color-popover-border'],
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 6)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-popover-border']} 50%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-deep']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.1s',
    '--tw-duration': '.1s',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-pulldown-hover'],
      },
    },
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule5: {
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    overflow: 'auto',
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule6: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
  },
  rule8: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-error'],
  },
  rule9: {
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-error-border'],
    backgroundColor: themeColors['--color-error-bg'],
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    overflowWrap: 'break-word',
  },
  rule10: {
    maxHeight: `calc(${themeLayout['--spacing']} * 24)`,
    overflow: 'auto',
    borderRadius: '.25rem',
    backgroundColor: themeColors['--color-background-deep'],
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    overflowWrap: 'break-word',
    whiteSpace: 'pre-wrap',
    color: themeColors['--color-foreground-alt'],
  },
  rule11: {
    display: 'flex',
    flexShrink: 0,
    gap: themeLayout['--spacing'],
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-popover-border']} 50%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-deep']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule12: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule13: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

interface ResourceDetailsPanelProps {
  resource: TrackedResource

  sx?: stylex.StyleXStyles
  className?: never
}

// ResourceDetailsPanel displays detailed information about a selected resource.
export function ResourceDetailsPanel({
  resource,
  sx,
}: ResourceDetailsPanelProps) {
  const devtools = useResourceDevToolsContext()

  const handleRetry = useCallback(() => {
    resource.retry()
  }, [resource])

  const handleCopy = useCallback(() => {
    const data = {
      id: resource.id,
      resourceId: resource.resourceId,
      resourceType: resource.resourceType,
      state: resource.state,
      released: resource.released,
      debugLabel: resource.debugLabel,
      debugDetails: resource.debugDetails,
      error: resource.error
        ? {
            message: resource.error.message,
            stack: resource.error.stack,
          }
        : null,
      createdAt: new Date(resource.createdAt).toISOString(),
      parentIds: resource.parentIds,
    }
    void navigator.clipboard.writeText(JSON.stringify(data, null, 2))
  }, [resource])

  const handleClose = useCallback(() => {
    devtools?.setSelectedId(null)
  }, [devtools])

  const createdAtStr = useMemo(() => {
    const date = new Date(resource.createdAt)
    return date.toLocaleTimeString()
  }, [resource.createdAt])

  // Live-updating duration for loading resources
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    if (resource.state !== 'loading') return
    const interval = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(interval)
  }, [resource.state])

  const durationStr = useMemo(() => {
    const elapsed = now - resource.createdAt
    const secs = Math.floor(elapsed / 1000)
    if (secs < 60) return `${secs}s`
    const mins = Math.floor(secs / 60)
    const remainingSecs = secs % 60
    return `${mins}m ${remainingSecs}s`
  }, [resource.createdAt, now])

  const stateColor = useMemo(() => {
    if (resource.state === 'ready')
      return [stylexStyles3.rule1, resourceTones.success]
    if (resource.state === 'loading')
      return [stylexStyles3.rule1, resourceTones.warning]
    if (resource.state === 'error')
      return [stylexStyles3.rule1, resourceTones.error]
    return [stylexStyles3.rule2]
  }, [resource.state])

  return (
    <div sx={[[stylexStyles.rule1], sx]} data-testid="resource-details-panel">
      {/* Header */}
      <div sx={[stylexStyles.rule2]}>
        <span sx={stylexStyles2.rule1}>Details</span>
        <button type="button" onClick={handleClose} sx={[stylexStyles.rule3]}>
          <LuX sx={stylexStyles.rule4} />
        </button>
      </div>

      {/* Content */}
      <div sx={stylexStyles.rule5}>
        {/* Type & ID */}
        <div sx={stylexStyles.rule6}>
          <DetailRow
            label="Type"
            value={resource.resourceType ?? '(unknown)'}
            mono
          />
          <DetailRow
            label="ID"
            value={
              resource.resourceId != null ? `#${resource.resourceId}` : '(none)'
            }
            mono
          />
        </div>

        {/* Status */}
        <div sx={stylexStyles.rule6}>
          <DetailRow
            label="State"
            value={resource.state}
            valueSx={stateColor}
          />
          {resource.state === 'loading' && (
            <DetailRow
              label="Duration"
              value={durationStr}
              valueSx={[[stylexStyles3.rule1], resourceTones.warning]}
            />
          )}
          <DetailRow
            label="Released"
            value={resource.released ? 'Yes' : 'No'}
            valueSx={
              resource.released
                ? [stylexStyles3.rule1, resourceTones.muted]
                : undefined
            }
          />
          <DetailRow label="Created" value={createdAtStr} />
        </div>

        {/* Debug Details */}
        {resource.debugDetails &&
          Object.keys(resource.debugDetails).length > 0 && (
            <div sx={stylexStyles.rule6}>
              <span sx={stylexStyles.rule7}>Debug Info</span>
              {Object.entries(resource.debugDetails).map(([key, value]) => (
                <DetailRow
                  key={key}
                  label={key}
                  value={value != null ? String(value) : '(null)'}
                  mono
                />
              ))}
            </div>
          )}

        {/* Error section */}
        {resource.error && (
          <div sx={stylexStyles.rule6}>
            <span sx={stylexStyles.rule8}>Error</span>
            <div sx={[stylexStyles.rule9]}>{resource.error.message}</div>
            {resource.error.stack && (
              <pre sx={[stylexStyles.rule10]}>{resource.error.stack}</pre>
            )}
          </div>
        )}
      </div>

      {/* Actions footer */}
      <div sx={[stylexStyles.rule11]}>
        <ActionButton
          onClick={handleRetry}
          icon={<LuRefreshCw sx={actionIcons.small} />}
        >
          Retry
        </ActionButton>
        <ActionButton
          onClick={handleCopy}
          icon={<LuCopy sx={actionIcons.small} />}
        >
          Copy
        </ActionButton>
      </div>
    </div>
  )
}

interface DetailRowProps {
  label: string
  value: string
  mono?: boolean
  valueSx?: stylex.StyleXStyles
}

function DetailRow({ label, value, mono, valueSx }: DetailRowProps) {
  return (
    <div sx={stylexStyles.rule12}>
      <span sx={stylexStyles.rule13}>{label}</span>
      <span
        sx={[
          [stylexStyles4.rule1, !!mono && stylexStyles4.rule2, false],
          [valueSx],
        ]}
      >
        {value}
      </span>
    </div>
  )
}

interface ActionButtonProps {
  onClick: () => void
  icon: React.ReactNode
  children: React.ReactNode
}

function ActionButton({ onClick, icon, children }: ActionButtonProps) {
  return (
    <button type="button" onClick={onClick} sx={[stylexStyles2.rule2]}>
      <span>{icon}</span>
      {children}
    </button>
  )
}

const actionIcons = stylex.create({ small: { width: 12, height: 12 } })

const resourceTones = stylex.create({
  success: { color: 'var(--color-success)' },
  warning: { color: 'var(--color-warning)' },
  error: { color: 'var(--color-error)' },
  muted: { color: 'var(--color-text-muted)' },
})
