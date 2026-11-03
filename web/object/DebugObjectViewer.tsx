import { useCallback } from 'react'
import { LuBug, LuCircleAlert } from '@s4wave/web/images/icons.js'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { getObjectType } from '@s4wave/sdk/world/types/types.js'

import type { ObjectViewerComponentProps } from './object.js'
import { getObjectKey, getTypeID } from './object.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  radii as themeRadii,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { marginInline: 'auto', maxWidth: 'var(--container-3xl)' },
})

const childSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    overflow: 'auto',
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule4: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule6: {
    flex: 1,
    overflow: 'auto',
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: {
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    display: 'flex',
  },
  rule8: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule9: { flex: 1, minWidth: 0 },
  rule10: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule11: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule12: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: 'flex',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule13: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule14: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule15: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule16: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule17: {
    '--tw-leading': 'var(--leading-relaxed)',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    overflow: 'auto',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
})

type WorldStateValue = ObjectViewerComponentProps['worldState']['value']

// DebugObjectViewer displays debug information about an object.
// Registered as the wildcard viewer ('*'), so it also handles untyped world
// objects (objects that exist in the world graph without a type predicate).
export function DebugObjectViewer({
  sx,
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const objectKey = getObjectKey(objectInfo)
  const hintTypeID = getTypeID(objectInfo)

  const resolveTypeID = useCallback(
    async (world: WorldStateValue, signal: AbortSignal) => {
      if (!world || !objectKey) return ''
      return getObjectType(world, objectKey, signal)
    },
    [objectKey],
  )

  const resolvedTypeIDResource = useResource(worldState, resolveTypeID, [
    objectKey,
  ])
  const resolvedTypeID = resolvedTypeIDResource.value
  const resolving = resolvedTypeIDResource.loading

  const effectiveTypeID = resolvedTypeID ?? hintTypeID
  const untyped = !resolving && !effectiveTypeID

  return (
    <div sx={[stylexStyles.rule1, sx]}>
      <div sx={stylexStyles.rule2}>
        <div sx={stylexStyles.rule3}>
          <LuBug sx={stylexStyles.rule4} />
          <span sx={stylexStyles.rule5}>Debug Viewer</span>
        </div>
      </div>

      <div sx={stylexStyles.rule6}>
        <div sx={stylexStyles2.rule1}>
          {untyped && (
            <div sx={[stylexStyles.rule7, childSpacing.s3]}>
              <div sx={stylexStyles.rule8}>
                <LuCircleAlert sx={stylexStyles.rule4} />
              </div>
              <div sx={stylexStyles.rule9}>
                <p sx={stylexStyles.rule10}>Object has no type</p>
                <p sx={stylexStyles.rule11}>
                  This object exists in the world graph without a type
                  predicate, so no specialized viewer is available. The debug
                  viewer is showing raw metadata below.
                </p>
              </div>
            </div>
          )}

          <section sx={childSpacing.s3}>
            <div sx={stylexStyles.rule12}>
              <h2 sx={stylexStyles.rule13}>
                <LuBug sx={stylexStyles.rule14} />
                Object Info
              </h2>
            </div>

            <InfoCard>
              <div>
                <CopyableField
                  sx={childSpacing.s2}
                  label="Object Key"
                  value={objectKey || '(none)'}
                />
                <CopyableField
                  sx={childSpacing.s2}
                  label="Type ID (resolved)"
                  value={
                    resolving
                      ? 'Resolving...'
                      : effectiveTypeID
                        ? effectiveTypeID
                        : '(untyped)'
                  }
                />
                {hintTypeID && hintTypeID !== resolvedTypeID ? (
                  <CopyableField
                    sx={childSpacing.s2}
                    label="Type ID (route hint)"
                    value={hintTypeID}
                  />
                ) : null}
              </div>
            </InfoCard>
          </section>

          <section sx={childSpacing.s3}>
            <div sx={stylexStyles.rule12}>
              <h2 sx={stylexStyles.rule15}>Object State</h2>
            </div>

            <div sx={stylexStyles.rule16}>
              <pre sx={stylexStyles.rule17}>
                {JSON.stringify(
                  {
                    key: objectState?.getKey() ?? null,
                    hasState: !!objectState,
                  },
                  null,
                  2,
                )}
              </pre>
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}
