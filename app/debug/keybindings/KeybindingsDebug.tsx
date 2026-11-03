import { utilityState as dividerState } from '@s4wave/web/style/tokens.stylex.js'
import { useCallback, useMemo, useState } from 'react'
import {
  LuChartNoAxesColumnIncreasing,
  LuCommand,
  LuKeyboard,
  LuLayoutList,
  LuRotateCcw,
  LuRows3,
  LuTriangleAlert,
} from '@s4wave/web/images/icons.js'

import { useNavigate } from '@s4wave/web/router/router.js'

import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { Badge } from '@s4wave/web/ui/badge.js'
import { Button } from '@s4wave/web/ui/button.js'
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@s4wave/web/ui/tabs.js'

import { CommandFinderVariant } from './CommandFinderVariant.js'
import { FlatTableVariant } from './FlatTableVariant.js'
import { GroupedPanelsVariant } from './GroupedPanelsVariant.js'
import { KeyboardMapVariant } from './KeyboardMapVariant.js'
import { ShortcutAuditVariant } from './ShortcutAuditVariant.js'
import { useKeybindingsPrototype } from './useKeybindingsPrototype.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

import { radii as themeRadii } from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    marginTop: `calc(${themeLayout['--spacing']} * -3)`,
    display: { default: 'none', '@container (min-width: 56rem)': 'grid' },
    gridTemplateColumns: 'repeat(5, minmax(0, 1fr))',
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 2%, transparent)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-7xl)',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    containerType: 'inline-size',
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background'],
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    textTransform: 'uppercase',
  },
  rule4: {
    flex: 1,
    overflow: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 6)`,
    },
  },
  rule5: {
    display: 'flex',
    flexDirection: {
      default: 'column',
      '@container (min-width: 32rem)': 'row',
    },
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: { default: null, '@container (min-width: 32rem)': 'flex-end' },
    justifyContent: {
      default: null,
      '@container (min-width: 32rem)': 'space-between',
    },
  },
  rule6: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule7: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: 'var(--text-2xl)',
    lineHeight: 'var(--tw-leading, var(--text-2xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule8: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    maxWidth: 'var(--container-3xl)',
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
  },
  rule9: { overflowX: 'auto', paddingBottom: themeLayout['--spacing'] },
  rule10: {
    display: { default: null, '@container not (min-width: 32rem)': 'none' },
  },
  rule11: {
    display: { default: null, '@container (min-width: 32rem)': 'none' },
  },
  rule12: {
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule13: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  rule14: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
      },
    },
  },
  rule15: {
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule16: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    display: 'block',
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
  },
})

type VariantId = 'table' | 'collections' | 'finder' | 'keyboard' | 'audit'

const VARIANTS: readonly {
  id: VariantId
  label: string
  shortLabel: string
  description: string
  icon: typeof LuLayoutList
}[] = [
  {
    id: 'table',
    label: 'Inventory table',
    shortLabel: 'Table',
    description: 'Fast scanning and inline editing',
    icon: LuLayoutList,
  },
  {
    id: 'collections',
    label: 'Collections',
    shortLabel: 'Groups',
    description: 'Category and context-first browsing',
    icon: LuRows3,
  },
  {
    id: 'finder',
    label: 'Command finder',
    shortLabel: 'Finder',
    description: 'Search-first focused editing',
    icon: LuCommand,
  },
  {
    id: 'keyboard',
    label: 'Keyboard atlas',
    shortLabel: 'Keyboard',
    description: 'Spatial shortcut discovery',
    icon: LuKeyboard,
  },
  {
    id: 'audit',
    label: 'Health workbench',
    shortLabel: 'Audit',
    description: 'Conflict and coverage triage',
    icon: LuChartNoAxesColumnIncreasing,
  },
]

export function KeybindingsDebug() {
  const navigate = useNavigate()
  const [variant, setVariant] = useState<VariantId>('table')
  const prototype = useKeybindingsPrototype()

  const goBack = useCallback(() => {
    navigate({ path: '/debug' })
  }, [navigate])

  const changeVariant = useCallback((value: string) => {
    setVariant(value as VariantId)
  }, [])

  const variantProps = useMemo(
    () => ({
      commands: prototype.commands,
      conflictCommandIds: prototype.conflictCommandIds,
      customizedCommandIds: prototype.customizedCommandIds,
      setBinding: prototype.setBinding,
      resetBinding: prototype.resetBinding,
    }),
    [
      prototype.commands,
      prototype.conflictCommandIds,
      prototype.customizedCommandIds,
      prototype.resetBinding,
      prototype.setBinding,
    ],
  )

  return (
    <div sx={stylexStyles.rule1}>
      <header sx={stylexStyles.rule2}>
        <BackButton onClick={goBack}>Debug gallery</BackButton>
        <span sx={stylexStyles.rule3}>
          Local prototype · changes reset on reload
        </span>
      </header>

      <main sx={stylexStyles.rule4}>
        <div sx={stylexStyles2.rule1}>
          <section sx={[nativeChildSpacing.s6, stylexStyles.rule5]}>
            <div>
              <div sx={stylexStyles.rule6}>
                <Badge variant="brand">UI exploration</Badge>
                <span sx={stylexStyles.rule3}>Keyboard shortcuts</span>
              </div>
              <h1 sx={stylexStyles.rule7}>
                Five ways to make keybindings feel approachable
              </h1>
              <p sx={stylexStyles.rule8}>
                Each option uses the same live in-memory command set. Rebind a
                command in one view, then switch variants to see conflicts and
                customizations update everywhere.
              </p>
            </div>

            <div sx={stylexStyles.rule6}>
              <Badge variant="subtle">
                {prototype.commands.length} commands
              </Badge>
              <Badge
                variant={prototype.conflictCount > 0 ? 'conflict' : 'subtle'}
              >
                <LuTriangleAlert /> {prototype.conflictCount} conflicting
              </Badge>
              <Badge variant="muted">
                {prototype.customizedCommandIds.size} customized
              </Badge>
              {prototype.customizedCommandIds.size > 0 ? (
                <Button
                  type="button"
                  variant="muted"
                  onClick={prototype.resetAllBindings}
                >
                  <LuRotateCcw /> Reset all
                </Button>
              ) : null}
            </div>
          </section>

          <Tabs
            sx={nativeChildSpacing.s6}
            value={variant}
            onValueChange={changeVariant}
            variant="debug"
          >
            <div sx={stylexStyles.rule9}>
              <TabsList variant="debug">
                {VARIANTS.map((option) => {
                  const Icon = option.icon
                  return (
                    <TabsTrigger
                      key={option.id}
                      value={option.id}
                      variant="debug"
                    >
                      <Icon />
                      <span sx={stylexStyles.rule10}>{option.label}</span>
                      <span sx={stylexStyles.rule11}>{option.shortLabel}</span>
                    </TabsTrigger>
                  )
                })}
              </TabsList>
            </div>

            <div sx={stylexStyles3.rule1}>
              {VARIANTS.map((option) => (
                <button
                  key={option.id}
                  type="button"
                  sx={[
                    [
                      stylexStyles.rule12,
                      variant === option.id
                        ? stylexStyles.rule13
                        : stylexStyles.rule14,
                    ],
                    nativeColumnDividers.t8,
                  ]}
                  onClick={() => setVariant(option.id)}
                >
                  <span sx={stylexStyles.rule15}>{option.label}</span>
                  <span sx={stylexStyles.rule16}>{option.description}</span>
                </button>
              ))}
            </div>

            <TabsContent value="table">
              <FlatTableVariant {...variantProps} />
            </TabsContent>
            <TabsContent value="collections">
              <GroupedPanelsVariant {...variantProps} />
            </TabsContent>
            <TabsContent value="finder">
              <CommandFinderVariant {...variantProps} />
            </TabsContent>
            <TabsContent value="keyboard">
              <KeyboardMapVariant {...variantProps} />
            </TabsContent>
            <TabsContent value="audit">
              <ShortcutAuditVariant {...variantProps} />
            </TabsContent>
          </Tabs>
        </div>
      </main>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s6: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 6 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 6 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeColumnDividers applies the list border only to its nonfinal physical columns.
const nativeColumnDividers = stylex.create({
  t8: {
    '--tw-divide-x-reverse': { default: null, ':not(:last-child)': 0 },
    borderInlineEndStyle: {
      default: null,
      ':not(:last-child)': dividerState['--tw-border-style'],
    },
    borderInlineStartStyle: {
      default: null,
      ':not(:last-child)': dividerState['--tw-border-style'],
    },
    borderInlineStartWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * var(--tw-divide-x-reverse))',
    },
    borderInlineEndWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * calc(1 - var(--tw-divide-x-reverse)))',
    },
    borderColor: {
      default: null,
      ':not(:last-child)':
        'color-mix(in oklab, var(--color-foreground) 8%, transparent)',
    },
  },
})
