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

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    overflow: 'auto',
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    display: 'flex',
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
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule6: {
    flex: 1,
    overflow: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule7: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule8: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule9: { minWidth: 0, flex: 1 },
  rule10: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule11: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule12: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule13: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule14: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule15: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule16: {
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule17: {
    overflow: 'auto',
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
  },
})

type WorldStateValue = ObjectViewerComponentProps['worldState']['value']

// DebugObjectViewer displays debug information about an object.
// Registered as the wildcard viewer ('*'), so it also handles untyped world
// objects (objects that exist in the world graph without a type predicate).
export function DebugObjectViewer({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
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
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        <div sx={stylexStyles.rule3}>
          <LuBug {...stylex.props(stylexStyles.rule4)} />
          <span sx={stylexStyles.rule5}>Debug Viewer</span>
        </div>
      </div>

      <div sx={stylexStyles.rule6}>
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'mx-auto max-w-3xl space-y-3'
          }
        >
          {untyped && (
            <div sx={stylexStyles.rule7}>
              <div sx={stylexStyles.rule8}>
                <LuCircleAlert {...stylex.props(stylexStyles.rule4)} />
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

          <section>
            <div sx={stylexStyles.rule12}>
              <h2 sx={stylexStyles.rule13}>
                <LuBug {...stylex.props(stylexStyles.rule14)} />
                Object Info
              </h2>
            </div>

            <InfoCard>
              <div
                className={
                  /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
                }
              >
                <CopyableField
                  label="Object Key"
                  value={objectKey || '(none)'}
                />
                <CopyableField
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
                    label="Type ID (route hint)"
                    value={hintTypeID}
                  />
                ) : null}
              </div>
            </InfoCard>
          </section>

          <section>
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
