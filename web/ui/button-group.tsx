import { useControlStyles } from '@s4wave/web/style/ControlStylesProvider.js'

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
    gap: {
      default: null,
      ':has( > [data-slot="button-group"])': `calc(${themeLayout['--spacing']} * 2)`,
    },
    alignItems: 'stretch',
    display: 'flex',
    width: 'fit-content',
  },
  rule2: {
    gap: {
      default: null,
      ':has( > [data-slot="button-group"])': `calc(${themeLayout['--spacing']} * 2)`,
    },
    alignItems: 'stretch',
    display: 'flex',
    flexDirection: 'column',
    width: 'fit-content',
  },
  rule3: {
    gap: {
      default: null,
      ':has( > [data-slot="button-group"])': `calc(${themeLayout['--spacing']} * 2)`,
    },
    alignItems: 'stretch',
    display: 'flex',
    width: 'fit-content',
  },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: colors['--color-muted'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
})

const separatorStyles = stylex.create({
  base: {
    margin: 0,
    alignSelf: 'stretch',
    backgroundColor: colors['--color-input'],
    position: 'relative',
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
  React.ComponentProps<'div'> &
    NonNullable<Parameters<typeof buttonGroupVariants>[0]>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  const attributes = stylexProps(
    [buttonGroupVariants({ orientation }), sx],
    props,
  )
  const inheritedClass = useControlStyles()
  attributes.className = [attributes.className, groupClass, inheritedClass]
    .filter(Boolean)
    .join(' ')
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
  attributes.className = [attributes.className, groupClass, inheritedClass]
    .filter(Boolean)
    .join(' ')

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
