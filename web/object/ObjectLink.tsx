import { useCallback, useMemo, useState } from 'react'
import { LuCheck, LuCopy, LuExternalLink } from '@s4wave/web/images/icons.js'

import type { AddTabRequest } from '@s4wave/sdk/layout/layout.pb.js'
import {
  createObjectLayoutAddTabRequest,
  createObjectLayoutObjectInfo,
} from '@s4wave/sdk/layout/world/object-layout.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import {
  getObjectTypeIcon,
  getObjectTypeLabel,
} from '@s4wave/web/space/object-tree.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

import type { ObjectInfo } from './object.pb.js'
import { useTabContext } from './TabContext.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  motion as themeMotion,
  colors as themeColors,
  fonts as themeFonts,
  utilityState as themeUtilityState,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'inline-flex',
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderTopLeftRadius: themeRadii['--radius-md'],
    borderBottomLeftRadius: themeRadii['--radius-md'],
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule2: {
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule3: {
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
  },
  rule4: { cursor: 'not-allowed' },
  rule5: {
    color: themeColors['--color-foreground'],
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule6: {
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule7: { minWidth: 0 },
  rule8: {
    display: 'block',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: '.6rem',
    lineHeight: `calc(${themeLayout['--spacing']} * 4)`,
    '--tw-leading': `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule9: {
    display: 'block',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: '.55rem',
    lineHeight: `calc(${themeLayout['--spacing']} * 3)`,
    '--tw-leading': `calc(${themeLayout['--spacing']} * 3)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule10: {
    display: 'inline-flex',
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
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
  rule11: {
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule12: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule13: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-success'],
  },
  rule14: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule15: {
    display: 'inline-flex',
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderTopRightRadius: themeRadii['--radius-md'],
    borderBottomRightRadius: themeRadii['--radius-md'],
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
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
        ':disabled:hover': '#0000',
      },
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    color: {
      default: null,
      ':disabled': `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
    },
  },
})

export interface ObjectLinkProps {
  objectKey?: string
  objectType?: string
  label?: string
  kind?: string
  status?: string
  componentID?: string
  path?: string
  className?: string
  compact?: boolean
}

export interface ObjectLinkTarget {
  objectKey: string
  objectType?: string
  label?: string
  componentID?: string
  path?: string
}

// useObjectLinkActions binds the open, copy, and open in new tab actions of an
// object link. Opening adds a tab inside an object layout and otherwise
// navigates the space container.
function useObjectLinkActions({
  targetKey,
  objectType,
  label,
  componentID,
  path,
}: ObjectLinkTarget & { targetKey: string }) {
  // Bind navigation and copy feedback to the current reference.
  const spaceContainer = SpaceContainerContext.useContextSafe()
  const tabContext = useTabContext()
  const [copied, setCopied] = useState(false)

  // Open the reference in its existing layout or the current Space route.
  const open = useCallback(() => {
    if (!targetKey) return
    if (tabContext?.isObjectLayout) {
      void tabContext
        .addTab(
          createObjectLinkAddTabRequest({
            objectKey: targetKey,
            objectType,
            label,
            componentID,
            path,
            currentTabId: tabContext.tabId,
          }),
        )
        .catch((err) => console.warn('ObjectLink: failed to add tab:', err))
      return
    }
    spaceContainer?.navigateToObjects([targetKey])
  }, [
    componentID,
    label,
    objectType,
    path,
    spaceContainer,
    tabContext,
    targetKey,
  ])

  // Report copy success only after the clipboard accepts the key.
  const copy = useCallback(() => {
    if (!targetKey) return
    void navigator.clipboard
      .writeText(targetKey)
      .then(() => {
        setCopied(true)
        setTimeout(() => setCopied(false), 2000)
      })
      .catch((err) =>
        console.warn('ObjectLink: failed to copy object key:', err),
      )
  }, [targetKey])

  // Prefer an external object URL when the Space can provide one.
  const openInNewTab = useCallback(() => {
    if (!targetKey) return
    const url = spaceContainer?.buildObjectUrls([targetKey])[0]
    if (url) {
      window.open(url, '_blank', 'noopener,noreferrer')
      return
    }
    spaceContainer?.navigateToObjects([targetKey])
  }, [spaceContainer, targetKey])

  return {
    copied,
    open,
    copy,
    openInNewTab,
    canOpenInNewTab: Boolean(
      targetKey && spaceContainer?.buildObjectUrls([targetKey])[0],
    ),
    hasSpaceContainer: !!spaceContainer,
  }
}

// ObjectLinkPrimary renders the button that opens the object, with its icon,
// label, and kind.
function ObjectLinkPrimary({
  objectType,
  displayLabel,
  accessibleLabel,
  detail,
  missing,
  compact,
  onOpen,
}: {
  objectType?: string
  displayLabel: string
  accessibleLabel: string
  detail: string
  missing: boolean
  compact: boolean
  onOpen: () => void
}) {
  return (
    <button
      type="button"
      disabled={missing}
      aria-label={'Open ' + accessibleLabel}
      onClick={onOpen}
      sx={[
        stylexStyles.rule1,
        compact ? stylexStyles.rule2 : stylexStyles.rule3,
        missing ? stylexStyles.rule4 : stylexStyles.rule5,
      ]}
    >
      <span sx={stylexStyles.rule6}>{getObjectTypeIcon(objectType ?? '')}</span>
      <span sx={stylexStyles.rule7}>
        <span sx={stylexStyles.rule8}>
          {missing ? displayLabel || 'Missing ref' : displayLabel}
        </span>
        {detail && !compact ? (
          <span sx={stylexStyles.rule9}>{detail}</span>
        ) : null}
      </span>
    </button>
  )
}

// ObjectLinkSecondaryActions renders the copy and new-tab actions of a resolved reference.
function ObjectLinkSecondaryActions({
  actions,
  accessibleLabel,
  compact,
}: {
  actions: ReturnType<typeof useObjectLinkActions>
  accessibleLabel: string
  compact: boolean
}) {
  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label={'Copy ' + accessibleLabel}
            onClick={actions.copy}
            sx={[
              stylexStyles.rule10,
              compact ? stylexStyles.rule11 : stylexStyles.rule12,
            ]}
          >
            {actions.copied ? (
              <LuCheck sx={stylexStyles.rule13} />
            ) : (
              <LuCopy sx={stylexStyles.rule14} />
            )}
          </button>
        </TooltipTrigger>
        <TooltipContent side="bottom">
          {actions.copied ? 'Copied' : 'Copy object key'}
        </TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label={'Open ' + accessibleLabel + ' in new tab'}
            disabled={!actions.canOpenInNewTab && !actions.hasSpaceContainer}
            onClick={actions.openInNewTab}
            sx={[
              stylexStyles.rule15,
              compact ? stylexStyles.rule11 : stylexStyles.rule12,
            ]}
          >
            <LuExternalLink sx={stylexStyles.rule14} />
          </button>
        </TooltipTrigger>
        <TooltipContent side="bottom">Open in new tab</TooltipContent>
      </Tooltip>
    </>
  )
}

// ObjectLink renders an object reference that opens, copies, or opens the
// object in a new tab.
export function ObjectLink({
  objectKey,
  objectType,
  label,
  kind,
  status,
  componentID,
  path,
  className,
  compact = false,
}: ObjectLinkProps) {
  // Bind actions to the normalized target before deriving its label.
  const targetKey = objectKey?.trim() ?? ''
  const missing = targetKey === ''
  const actions = useObjectLinkActions({
    targetKey,
    objectKey: targetKey,
    objectType,
    label,
    componentID,
    path,
  })

  // Describe the resolved or missing reference for both visible and accessible labels.
  const displayLabel = useMemo(
    () => objectLinkDisplayLabel({ objectKey: targetKey, label }),
    [targetKey, label],
  )
  const kindLabel = kind || (objectType ? getObjectTypeLabel(objectType) : '')
  const accessibleLabel = displayLabel || targetKey || 'missing object ref'

  return (
    <span
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'border-foreground/8 bg-foreground/5 text-foreground-alt/80 inline-flex max-w-full items-center rounded-md border align-middle',
          missing && 'border-warning/20 bg-warning/5 text-warning',
          compact ? 'min-h-6' : 'min-h-7',
          className,
        )
      }
      data-object-link={targetKey || undefined}
      data-missing-object-link={missing ? 'true' : undefined}
    >
      <ObjectLinkPrimary
        objectType={objectType}
        displayLabel={displayLabel}
        accessibleLabel={accessibleLabel}
        detail={[kindLabel, status].filter(Boolean).join(' / ')}
        missing={missing}
        compact={compact}
        onOpen={actions.open}
      />
      {missing ? null : (
        <ObjectLinkSecondaryActions
          actions={actions}
          accessibleLabel={accessibleLabel}
          compact={compact}
        />
      )}
    </span>
  )
}

export function createObjectLinkAddTabRequest(
  target: ObjectLinkTarget & { currentTabId?: string },
): AddTabRequest {
  return createObjectLayoutAddTabRequest({
    afterTabId: target.currentTabId,
    select: true,
    id: objectLinkTabId(target),
    name: objectLinkDisplayLabel(target),
    objectKey: target.objectKey,
    objectType: target.objectType,
    componentID: target.componentID,
    path: target.path,
    helpText: target.objectKey,
    enableClose: true,
  })
}

export function createObjectLinkObjectInfo(
  target: ObjectLinkTarget,
): ObjectInfo {
  return createObjectLayoutObjectInfo(target)
}

export function objectLinkTabId(target: ObjectLinkTarget): string {
  const raw = [
    target.objectKey,
    target.objectType,
    target.componentID,
    target.path,
  ]
    .filter(Boolean)
    .join('|')
  const slug =
    raw
      .toLowerCase()
      .replace(/[^a-z0-9]+/gu, '-')
      .replace(/^-+|-+$/gu, '')
      .slice(0, 80) || 'object'
  return 'object-link-' + slug + '-' + stableHash(raw)
}

// objectLinkDisplayLabel prefers an explicit label, then the key's final path segment.
export function objectLinkDisplayLabel(target: {
  objectKey?: string
  label?: string
}): string {
  const label = target.label?.trim()
  if (label) return label
  const key = target.objectKey?.trim()
  return key ? (key.split('/').filter(Boolean).at(-1) ?? key) : 'Missing ref'
}

function stableHash(value: string): string {
  const hash = Array.from(value).reduce(
    (acc, char) => Math.imul(acc ^ char.charCodeAt(0), 16777619),
    2166136261,
  )
  return (hash >>> 0).toString(36)
}
