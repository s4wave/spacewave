import { useControlStyles } from '@s4wave/web/style/ControlStylesProvider.js'
import { cn } from '@s4wave/web/style/utils.js'
import { buttonGroupControlStyles } from '../style/button-group.stylex.js'
import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { IconStylesProvider } from '@s4wave/web/images/icons.js'
import {
  iconDefaults,
  iconRequired,
  buttonGroupIconMarker,
} from '../style/icon-defaults.stylex.js'
import { Slot } from '@radix-ui/react-slot'
import { type VariantProps } from 'class-variance-authority'

import { Separator } from '@s4wave/web/ui/separator'
import * as stylex from '@stylexjs/stylex'
import { colors } from '../style/tokens.stylex.js'

import {
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'
import {
  buttonGroupMarker,
  buttonGroupVerticalMarker,
  buttonGroupHorizontalMarker,
} from '../style/relations.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    width: 'fit-content',
    alignItems: 'stretch',
    gap: {
      default: null,
      ':has( > [data-slot="button-group"])': `calc(${themeLayout['--spacing']} * 2)`,
    },
  },
  rule2: {
    display: 'flex',
    width: 'fit-content',
    flexDirection: 'column',
    alignItems: 'stretch',
    gap: {
      default: null,
      ':has( > [data-slot="button-group"])': `calc(${themeLayout['--spacing']} * 2)`,
    },
  },
  rule3: {
    display: 'flex',
    width: 'fit-content',
    alignItems: 'stretch',
    gap: {
      default: null,
      ':has( > [data-slot="button-group"])': `calc(${themeLayout['--spacing']} * 2)`,
    },
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: colors['--color-muted'],
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
})

const separatorStyles = stylex.create({
  base: {
    backgroundColor: colors['--color-input'],
    position: 'relative',
    margin: 0,
    alignSelf: 'stretch',
    height: {
      default: null,
      ':is([data-orientation="horizontal"])': '1px',
      ':is([data-orientation="vertical"])': 'auto',
    },
  },
})

/** buttonGroupVariants supplies group layout and DOM markers without adding an element. */
const buttonGroupVariants = (props?: {
  orientation?: 'horizontal' | 'vertical' | null
  sx?: stylex.StyleXStyles
}) => {
  // Resolve "orientation" once, retaining explicit null instead of its default.
  const variant0Input = props?.['orientation']
  const variant0 = variant0Input === undefined ? 'horizontal' : variant0Input

  // Select one fully compiled recipe outcome, then compose caller styles.
  return [
    variant0 === 'horizontal'
      ? [stylexStyles.rule3, buttonGroupMarker, buttonGroupHorizontalMarker]
      : variant0 === 'vertical'
        ? [stylexStyles.rule2, buttonGroupMarker, buttonGroupVerticalMarker]
        : [stylexStyles.rule1, buttonGroupMarker],
    props?.sx,
  ] as const
}

const groupClass = stylex.props(buttonGroupControlStyles.child).className

/** ButtonGroup joins native controls along one axis and retains their focus treatment. */
function ButtonGroup({
  sx,
  orientation,
  ...props
}: Omit<
  React.ComponentProps<'div'> & VariantProps<typeof buttonGroupVariants>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  const attributes = stylexProps(
    [buttonGroupVariants({ orientation }), sx],
    props,
  )
  const inheritedClass = useControlStyles()
  attributes.className = cn(attributes.className, groupClass, inheritedClass)
  return (
    <div
      role="group"
      data-slot="button-group"
      data-orientation={orientation}
      {...attributes}
    />
  )
}

/** ButtonGroupText renders a label alongside joined controls. */
function ButtonGroupText({
  sx,
  asChild = false,
  ...props
}: Omit<
  React.ComponentProps<'div'> & {
    asChild?: boolean
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  // Compose the label's native attributes before applying its independent control rules.
  const Comp = asChild ? Slot : 'div'
  const attributes = stylexProps(
    [buttonGroupIconMarker, stylexStyles.rule4, sx],
    props,
  )
  const inheritedClass = useControlStyles()
  attributes.className = cn(attributes.className, groupClass, inheritedClass)

  return (
    <IconStylesProvider
      sx={[iconDefaults.buttonGroup]}
      requiredSx={iconRequired.buttonGroup}
    >
      <Comp {...attributes} />
    </IconStylesProvider>
  )
}

/** ButtonGroupSeparator separates controls along the selected axis. */
function ButtonGroupSeparator({
  sx,
  orientation = 'vertical',
  ...props
}: React.ComponentProps<typeof Separator>) {
  return (
    <Separator
      data-slot="button-group-separator"
      orientation={orientation}
      sx={[separatorStyles.base, sx]}
      {...props}
    />
  )
}

export { ButtonGroup, ButtonGroupSeparator, ButtonGroupText }

export { buttonGroupVariants }
