import { keyframeNames } from '../style/tokens.stylex.js'
import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as stylex from '@stylexjs/stylex'
import { IconStylesProvider } from '@s4wave/web/images/icons.js'
import {
  iconDefaults,
  iconRequired,
  menuIconMarker,
  menuMutedIconMarker,
} from '../style/icon-defaults.stylex.js'
import * as React from 'react'
import * as DropdownMenuPrimitive from '@radix-ui/react-dropdown-menu'
import { LuCheck, LuChevronRight, LuCircle } from '@s4wave/web/images/icons.js'

import { layout as themeLayout } from '../style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const menuStyles = stylex.create({
  content: {
    '--tw-enter-opacity': { default: null, ':is([data-state="open"])': '0' },
    '--tw-enter-scale': { default: null, ':is([data-state="open"])': '.95' },
    '--tw-enter-translate-x': {
      default: null,
      ':is([data-side="left"])': '.5rem',
      ':is([data-side="right"])': '-.5rem',
    },
    '--tw-enter-translate-y': {
      default: null,
      ':is([data-side="bottom"])': '-.5rem',
      ':is([data-side="top"])': '.5rem',
    },
    '--tw-exit-opacity': { default: null, ':is([data-state="closed"])': '0' },
    '--tw-exit-scale': { default: null, ':is([data-state="closed"])': '.95' },
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    padding: themeLayout['--spacing'],
    borderColor: themeColors['--color-popover-border'],
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    animationDelay: {
      default: null,
      ':is([data-state="closed"])': '0s',
      ':is([data-state="open"])': '0s',
    },
    animationDirection: {
      default: null,
      ':is([data-state="closed"])': 'normal',
      ':is([data-state="open"])': 'normal',
    },
    animationDuration: {
      default: null,
      ':is([data-state="closed"])': '.15s',
      ':is([data-state="open"])': '.15s',
    },
    animationFillMode: {
      default: null,
      ':is([data-state="closed"])': 'both',
      ':is([data-state="open"])': 'both',
    },
    animationIterationCount: {
      default: null,
      ':is([data-state="closed"])': 1,
      ':is([data-state="open"])': 1,
    },
    animationName: {
      default: null,
      ':is([data-state="closed"])': keyframeNames['shadcn-exit'],
      ':is([data-state="open"])': keyframeNames['shadcn-enter'],
    },
    animationPlayState: {
      default: null,
      ':is([data-state="closed"])': 'running',
      ':is([data-state="open"])': 'running',
    },
    animationTimeline: {
      default: null,
      ':is([data-state="closed"])': 'auto',
      ':is([data-state="open"])': 'auto',
    },
    animationTimingFunction: {
      default: null,
      ':is([data-state="closed"])': 'ease-in',
      ':is([data-state="open"])': 'ease-out',
    },
    backgroundColor: themeColors['--color-popover'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    color: themeColors['--color-foreground'],
    transformOrigin: 'var(--radix-dropdown-menu-content-transform-origin)',
    zIndex: 50,
    maxHeight: 'var(--radix-dropdown-menu-content-available-height)',
    minWidth: '8rem',
    overflowX: 'hidden',
    overflowY: 'auto',
  },
  compactContent: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  selectedItem: {
    '--tw-outline-style': 'none',
    WebkitUserSelect: 'none',
    borderRadius: 'var(--radius-sm)',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    backgroundColor: {
      default: themeColors['--color-accent'],
      ':is(.dark *)': {
        default: null,
        ':is([data-variant="destructive"])': {
          default: null,
          ':focus': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
        },
      },
      ':is([data-variant="destructive"])': {
        default: null,
        ':focus': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
      ':focus': themeColors['--color-accent'],
    },
    color: {
      default: themeColors['--color-accent-foreground'],
      ':is([data-variant="destructive"])': {
        default: themeColors['--color-destructive'],
        ':focus': themeColors['--color-destructive'],
      },
      ':focus': themeColors['--color-accent-foreground'],
    },
    cursor: 'default',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: { default: null, ':is([data-disabled])': 0.5 },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    pointerEvents: { default: null, ':is([data-disabled])': 'none' },
    position: 'relative',
    userSelect: 'none',
    paddingLeft: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      ':is([data-inset])': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
  },
  item: {
    '--tw-outline-style': 'none',
    WebkitUserSelect: 'none',
    borderRadius: 'var(--radius-sm)',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      ':is(.dark *)': {
        default: null,
        ':is([data-variant="destructive"])': {
          default: null,
          ':focus': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
        },
      },
      ':is([data-variant="destructive"])': {
        default: null,
        ':focus': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
      ':focus': themeColors['--color-accent'],
    },
    color: {
      default: null,
      ':is([data-variant="destructive"])': {
        default: themeColors['--color-destructive'],
        ':focus': themeColors['--color-destructive'],
      },
      ':focus': themeColors['--color-accent-foreground'],
    },
    cursor: 'default',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: { default: null, ':is([data-disabled])': 0.5 },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    pointerEvents: { default: null, ':is([data-disabled])': 'none' },
    position: 'relative',
    userSelect: 'none',
    paddingLeft: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      ':is([data-inset])': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
  },
  choiceItem: {
    '--tw-outline-style': 'none',
    WebkitUserSelect: 'none',
    borderRadius: 'var(--radius-sm)',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    backgroundColor: { default: null, ':focus': themeColors['--color-accent'] },
    color: {
      default: null,
      ':focus': themeColors['--color-accent-foreground'],
    },
    cursor: 'default',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: { default: null, ':is([data-disabled])': 0.5 },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    pointerEvents: { default: null, ':is([data-disabled])': 'none' },
    position: 'relative',
    userSelect: 'none',
    paddingLeft: `calc(${themeLayout['--spacing']} * 8)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
  },
  label: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    paddingLeft: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      ':is([data-inset])': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
  },
  separator: {
    marginBlock: themeLayout['--spacing'],
    marginInline: `calc(${themeLayout['--spacing']} * -1)`,
    backgroundColor: themeColors['--color-border'],
    height: '1px',
  },
  shortcut: {
    '--tw-tracking': 'var(--tracking-widest)',
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-xs'],
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginLeft: 'auto',
  },
  subTrigger: {
    '--tw-outline-style': 'none',
    WebkitUserSelect: 'none',
    borderRadius: 'var(--radius-sm)',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      ':is([data-state="open"])': themeColors['--color-accent'],
      ':focus': themeColors['--color-accent'],
    },
    color: {
      default: null,
      ':is([data-state="open"])': themeColors['--color-accent-foreground'],
      ':focus': themeColors['--color-accent-foreground'],
    },
    cursor: 'default',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    userSelect: 'none',
    paddingLeft: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      ':is([data-inset])': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
  },
  subContent: {
    '--tw-enter-opacity': { default: null, ':is([data-state="open"])': '0' },
    '--tw-enter-scale': { default: null, ':is([data-state="open"])': '.95' },
    '--tw-enter-translate-x': {
      default: null,
      ':is([data-side="left"])': '.5rem',
      ':is([data-side="right"])': '-.5rem',
    },
    '--tw-enter-translate-y': {
      default: null,
      ':is([data-side="bottom"])': '-.5rem',
      ':is([data-side="top"])': '.5rem',
    },
    '--tw-exit-opacity': { default: null, ':is([data-state="closed"])': '0' },
    '--tw-exit-scale': { default: null, ':is([data-state="closed"])': '.95' },
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    padding: themeLayout['--spacing'],
    borderColor: themeColors['--color-popover-border'],
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    animationDelay: {
      default: null,
      ':is([data-state="closed"])': '0s',
      ':is([data-state="open"])': '0s',
    },
    animationDirection: {
      default: null,
      ':is([data-state="closed"])': 'normal',
      ':is([data-state="open"])': 'normal',
    },
    animationDuration: {
      default: null,
      ':is([data-state="closed"])': '.15s',
      ':is([data-state="open"])': '.15s',
    },
    animationFillMode: {
      default: null,
      ':is([data-state="closed"])': 'both',
      ':is([data-state="open"])': 'both',
    },
    animationIterationCount: {
      default: null,
      ':is([data-state="closed"])': 1,
      ':is([data-state="open"])': 1,
    },
    animationName: {
      default: null,
      ':is([data-state="closed"])': keyframeNames['shadcn-exit'],
      ':is([data-state="open"])': keyframeNames['shadcn-enter'],
    },
    animationPlayState: {
      default: null,
      ':is([data-state="closed"])': 'running',
      ':is([data-state="open"])': 'running',
    },
    animationTimeline: {
      default: null,
      ':is([data-state="closed"])': 'auto',
      ':is([data-state="open"])': 'auto',
    },
    animationTimingFunction: {
      default: null,
      ':is([data-state="closed"])': 'ease-in',
      ':is([data-state="open"])': 'ease-out',
    },
    backgroundColor: themeColors['--color-popover'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    color: themeColors['--color-foreground'],
    transformOrigin: 'var(--radix-dropdown-menu-content-transform-origin)',
    zIndex: 50,
    minWidth: '8rem',
  },
})

const indicatorStyles = stylex.create({
  indicator: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    pointerEvents: 'none',
    position: 'absolute',
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    left: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  checkIcon: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  radioIcon: {
    fill: 'currentColor',
    height: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 2)`,
  },
  submenuArrow: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    marginLeft: 'auto',
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

function DropdownMenu({
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.Root>) {
  return <DropdownMenuPrimitive.Root data-slot="dropdown-menu" {...props} />
}

function DropdownMenuPortal({
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.Portal>) {
  return (
    <DropdownMenuPrimitive.Portal data-slot="dropdown-menu-portal" {...props} />
  )
}

function DropdownMenuTrigger({
  sx,
  ...props
}: Omit<
  React.ComponentProps<typeof DropdownMenuPrimitive.Trigger>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <DropdownMenuPrimitive.Trigger
      data-slot="dropdown-menu-trigger"
      {...stylexProps(sx, props)}
    />
  )
}

function DropdownMenuContent({
  sx,
  variant,
  sideOffset = 4,
  ...props
}: Omit<
  React.ComponentProps<typeof DropdownMenuPrimitive.Content> & {
    variant?: 'default' | 'compact'
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <DropdownMenuPrimitive.Portal>
      <DropdownMenuPrimitive.Content
        data-slot="dropdown-menu-content"
        sideOffset={sideOffset}
        {...stylexProps(
          [
            menuStyles.content,
            variant === 'compact' && menuStyles.compactContent,
            sx,
          ],
          props,
        )}
      />
    </DropdownMenuPrimitive.Portal>
  )
}

function DropdownMenuGroup({
  sx,
  ...props
}: Omit<
  React.ComponentProps<typeof DropdownMenuPrimitive.Group>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <DropdownMenuPrimitive.Group
      data-slot="dropdown-menu-group"
      {...stylexProps(sx, props)}
    />
  )
}

function DropdownMenuItem({
  sx,
  inset,
  variant = 'default',
  ...props
}: Omit<
  React.ComponentProps<typeof DropdownMenuPrimitive.Item> & {
    inset?: boolean
    variant?: 'default' | 'destructive' | 'selected'
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <IconStylesProvider
      sx={[iconDefaults.menu, iconDefaults.menuMuted]}
      requiredSx={[iconRequired.menu, iconRequired.menuDestructive]}
    >
      <DropdownMenuPrimitive.Item
        data-slot="dropdown-menu-item"
        data-inset={inset}
        data-variant={variant}
        {...stylexProps(
          [
            menuIconMarker,
            menuMutedIconMarker,
            [
              variant === 'selected'
                ? menuStyles.selectedItem
                : menuStyles.item,
              sx,
            ],
          ],
          props,
        )}
      />
    </IconStylesProvider>
  )
}

function DropdownMenuCheckboxItem({
  sx,
  children,
  checked,
  ...props
}: Omit<
  React.ComponentProps<typeof DropdownMenuPrimitive.CheckboxItem>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <IconStylesProvider sx={[iconDefaults.menu]} requiredSx={iconRequired.menu}>
      <DropdownMenuPrimitive.CheckboxItem
        data-slot="dropdown-menu-checkbox-item"
        checked={checked}
        {...stylexProps([menuIconMarker, [menuStyles.choiceItem, sx]], props)}
      >
        <span sx={indicatorStyles.indicator}>
          <DropdownMenuPrimitive.ItemIndicator>
            <LuCheck sx={indicatorStyles.checkIcon} />
          </DropdownMenuPrimitive.ItemIndicator>
        </span>
        {children}
      </DropdownMenuPrimitive.CheckboxItem>
    </IconStylesProvider>
  )
}

function DropdownMenuRadioGroup({
  sx,
  ...props
}: Omit<
  React.ComponentProps<typeof DropdownMenuPrimitive.RadioGroup>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <DropdownMenuPrimitive.RadioGroup
      data-slot="dropdown-menu-radio-group"
      {...stylexProps(sx, props)}
    />
  )
}

function DropdownMenuRadioItem({
  sx,
  children,
  ...props
}: Omit<
  React.ComponentProps<typeof DropdownMenuPrimitive.RadioItem>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <IconStylesProvider sx={[iconDefaults.menu]} requiredSx={iconRequired.menu}>
      <DropdownMenuPrimitive.RadioItem
        data-slot="dropdown-menu-radio-item"
        {...stylexProps([menuIconMarker, [menuStyles.choiceItem, sx]], props)}
      >
        <span sx={indicatorStyles.indicator}>
          <DropdownMenuPrimitive.ItemIndicator>
            <LuCircle sx={indicatorStyles.radioIcon} />
          </DropdownMenuPrimitive.ItemIndicator>
        </span>
        {children}
      </DropdownMenuPrimitive.RadioItem>
    </IconStylesProvider>
  )
}

function DropdownMenuLabel({
  sx,
  inset,
  ...props
}: Omit<
  React.ComponentProps<typeof DropdownMenuPrimitive.Label> & {
    inset?: boolean
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <DropdownMenuPrimitive.Label
      data-slot="dropdown-menu-label"
      data-inset={inset}
      {...stylexProps([menuStyles.label, sx], props)}
    />
  )
}

function DropdownMenuSeparator({
  sx,
  ...props
}: Omit<
  React.ComponentProps<typeof DropdownMenuPrimitive.Separator>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <DropdownMenuPrimitive.Separator
      data-slot="dropdown-menu-separator"
      {...stylexProps([menuStyles.separator, sx], props)}
    />
  )
}

function DropdownMenuShortcut({
  sx,
  ...props
}: Omit<React.ComponentProps<'span'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <span
      data-slot="dropdown-menu-shortcut"
      {...stylexProps([menuStyles.shortcut, sx], props)}
    />
  )
}

function DropdownMenuSub({
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.Sub>) {
  return <DropdownMenuPrimitive.Sub data-slot="dropdown-menu-sub" {...props} />
}

function DropdownMenuSubTrigger({
  sx,
  inset,
  children,
  ...props
}: Omit<
  React.ComponentProps<typeof DropdownMenuPrimitive.SubTrigger> & {
    inset?: boolean
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <IconStylesProvider
      sx={[iconDefaults.menu, iconDefaults.menuMuted]}
      requiredSx={iconRequired.menu}
    >
      <DropdownMenuPrimitive.SubTrigger
        data-slot="dropdown-menu-sub-trigger"
        data-inset={inset}
        {...stylexProps(
          [menuIconMarker, menuMutedIconMarker, [menuStyles.subTrigger, sx]],
          props,
        )}
      >
        {children}
        <LuChevronRight sx={indicatorStyles.submenuArrow} />
      </DropdownMenuPrimitive.SubTrigger>
    </IconStylesProvider>
  )
}

function DropdownMenuSubContent({
  sx,
  ...props
}: Omit<
  React.ComponentProps<typeof DropdownMenuPrimitive.SubContent>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <DropdownMenuPrimitive.SubContent
      data-slot="dropdown-menu-sub-content"
      {...stylexProps([menuStyles.subContent, sx], props)}
    />
  )
}

export {
  DropdownMenu,
  DropdownMenuPortal,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuItem,
  DropdownMenuCheckboxItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuSub,
  DropdownMenuSubTrigger,
  DropdownMenuSubContent,
}
