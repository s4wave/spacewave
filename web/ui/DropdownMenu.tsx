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
    zIndex: 50,
    maxHeight: 'var(--radix-dropdown-menu-content-available-height)',
    minWidth: '8rem',
    transformOrigin: 'var(--radix-dropdown-menu-content-transform-origin)',
    overflowX: 'hidden',
    overflowY: 'auto',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-popover-border'],
    backgroundColor: themeColors['--color-popover'],
    padding: themeLayout['--spacing'],
    color: themeColors['--color-foreground'],
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-enter-translate-y': {
      default: null,
      ':is([data-side="bottom"])': '-.5rem',
      ':is([data-side="top"])': '.5rem',
    },
    '--tw-enter-translate-x': {
      default: null,
      ':is([data-side="left"])': '.5rem',
      ':is([data-side="right"])': '-.5rem',
    },
    animationName: {
      default: null,
      ':is([data-state="closed"])': 'shadcn-exit',
      ':is([data-state="open"])': 'shadcn-enter',
    },
    animationDuration: {
      default: null,
      ':is([data-state="closed"])': '.15s',
      ':is([data-state="open"])': '.15s',
    },
    animationTimingFunction: {
      default: null,
      ':is([data-state="closed"])': 'ease-in',
      ':is([data-state="open"])': 'ease-out',
    },
    animationIterationCount: {
      default: null,
      ':is([data-state="closed"])': 1,
      ':is([data-state="open"])': 1,
    },
    animationDirection: {
      default: null,
      ':is([data-state="closed"])': 'normal',
      ':is([data-state="open"])': 'normal',
    },
    animationPlayState: {
      default: null,
      ':is([data-state="closed"])': 'running',
      ':is([data-state="open"])': 'running',
    },
    animationDelay: {
      default: null,
      ':is([data-state="closed"])': '0s',
      ':is([data-state="open"])': '0s',
    },
    animationFillMode: {
      default: null,
      ':is([data-state="closed"])': 'both',
      ':is([data-state="open"])': 'both',
    },
    animationTimeline: {
      default: null,
      ':is([data-state="closed"])': 'auto',
      ':is([data-state="open"])': 'auto',
    },
    '--tw-exit-opacity': { default: null, ':is([data-state="closed"])': '0' },
    '--tw-exit-scale': { default: null, ':is([data-state="closed"])': '.95' },
    '--tw-enter-opacity': { default: null, ':is([data-state="open"])': '0' },
    '--tw-enter-scale': { default: null, ':is([data-state="open"])': '.95' },
  },
  compactContent: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  selectedItem: {
    position: 'relative',
    display: 'flex',
    cursor: 'default',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: 'var(--radius-sm)',
    backgroundColor: {
      default: themeColors['--color-accent'],
      ':focus': themeColors['--color-accent'],
      ':is([data-variant="destructive"])': {
        default: null,
        ':focus': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
      ':is(.dark *)': {
        default: null,
        ':is([data-variant="destructive"])': {
          default: null,
          ':focus': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
        },
      },
    },
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
    paddingLeft: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      ':is([data-inset])': `calc(${themeLayout['--spacing']} * 8)`,
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-accent-foreground'],
      ':focus': themeColors['--color-accent-foreground'],
      ':is([data-variant="destructive"])': {
        default: themeColors['--color-destructive'],
        ':focus': themeColors['--color-destructive'],
      },
    },
    '--tw-outline-style': 'none',
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    WebkitUserSelect: 'none',
    userSelect: 'none',
    pointerEvents: { default: null, ':is([data-disabled])': 'none' },
    opacity: { default: null, ':is([data-disabled])': 0.5 },
  },
  item: {
    position: 'relative',
    display: 'flex',
    cursor: 'default',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: 'var(--radius-sm)',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
    paddingLeft: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      ':is([data-inset])': `calc(${themeLayout['--spacing']} * 8)`,
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-outline-style': 'none',
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    WebkitUserSelect: 'none',
    userSelect: 'none',
    backgroundColor: {
      default: null,
      ':focus': themeColors['--color-accent'],
      ':is([data-variant="destructive"])': {
        default: null,
        ':focus': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
      ':is(.dark *)': {
        default: null,
        ':is([data-variant="destructive"])': {
          default: null,
          ':focus': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
        },
      },
    },
    color: {
      default: null,
      ':focus': themeColors['--color-accent-foreground'],
      ':is([data-variant="destructive"])': {
        default: themeColors['--color-destructive'],
        ':focus': themeColors['--color-destructive'],
      },
    },
    pointerEvents: { default: null, ':is([data-disabled])': 'none' },
    opacity: { default: null, ':is([data-disabled])': 0.5 },
  },
  choiceItem: {
    position: 'relative',
    display: 'flex',
    cursor: 'default',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: 'var(--radius-sm)',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 8)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-outline-style': 'none',
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    WebkitUserSelect: 'none',
    userSelect: 'none',
    backgroundColor: { default: null, ':focus': themeColors['--color-accent'] },
    color: {
      default: null,
      ':focus': themeColors['--color-accent-foreground'],
    },
    pointerEvents: { default: null, ':is([data-disabled])': 'none' },
    opacity: { default: null, ':is([data-disabled])': 0.5 },
  },
  label: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
    paddingLeft: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      ':is([data-inset])': `calc(${themeLayout['--spacing']} * 8)`,
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  separator: {
    marginInline: `calc(${themeLayout['--spacing']} * -1)`,
    marginBlock: themeLayout['--spacing'],
    height: '1px',
    backgroundColor: themeColors['--color-border'],
  },
  shortcut: {
    marginLeft: 'auto',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: themeColors['--color-muted-foreground'],
  },
  subTrigger: {
    display: 'flex',
    cursor: 'default',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: 'var(--radius-sm)',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
    paddingLeft: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      ':is([data-inset])': `calc(${themeLayout['--spacing']} * 8)`,
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-outline-style': 'none',
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    WebkitUserSelect: 'none',
    userSelect: 'none',
    backgroundColor: {
      default: null,
      ':focus': themeColors['--color-accent'],
      ':is([data-state="open"])': themeColors['--color-accent'],
    },
    color: {
      default: null,
      ':focus': themeColors['--color-accent-foreground'],
      ':is([data-state="open"])': themeColors['--color-accent-foreground'],
    },
  },
  subContent: {
    zIndex: 50,
    minWidth: '8rem',
    transformOrigin: 'var(--radix-dropdown-menu-content-transform-origin)',
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-popover-border'],
    backgroundColor: themeColors['--color-popover'],
    padding: themeLayout['--spacing'],
    color: themeColors['--color-foreground'],
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-enter-translate-y': {
      default: null,
      ':is([data-side="bottom"])': '-.5rem',
      ':is([data-side="top"])': '.5rem',
    },
    '--tw-enter-translate-x': {
      default: null,
      ':is([data-side="left"])': '.5rem',
      ':is([data-side="right"])': '-.5rem',
    },
    animationName: {
      default: null,
      ':is([data-state="closed"])': 'shadcn-exit',
      ':is([data-state="open"])': 'shadcn-enter',
    },
    animationDuration: {
      default: null,
      ':is([data-state="closed"])': '.15s',
      ':is([data-state="open"])': '.15s',
    },
    animationTimingFunction: {
      default: null,
      ':is([data-state="closed"])': 'ease-in',
      ':is([data-state="open"])': 'ease-out',
    },
    animationIterationCount: {
      default: null,
      ':is([data-state="closed"])': 1,
      ':is([data-state="open"])': 1,
    },
    animationDirection: {
      default: null,
      ':is([data-state="closed"])': 'normal',
      ':is([data-state="open"])': 'normal',
    },
    animationPlayState: {
      default: null,
      ':is([data-state="closed"])': 'running',
      ':is([data-state="open"])': 'running',
    },
    animationDelay: {
      default: null,
      ':is([data-state="closed"])': '0s',
      ':is([data-state="open"])': '0s',
    },
    animationFillMode: {
      default: null,
      ':is([data-state="closed"])': 'both',
      ':is([data-state="open"])': 'both',
    },
    animationTimeline: {
      default: null,
      ':is([data-state="closed"])': 'auto',
      ':is([data-state="open"])': 'auto',
    },
    '--tw-exit-opacity': { default: null, ':is([data-state="closed"])': '0' },
    '--tw-exit-scale': { default: null, ':is([data-state="closed"])': '.95' },
    '--tw-enter-opacity': { default: null, ':is([data-state="open"])': '0' },
    '--tw-enter-scale': { default: null, ':is([data-state="open"])': '.95' },
  },
})

const indicatorStyles = stylex.create({
  indicator: {
    pointerEvents: 'none',
    position: 'absolute',
    left: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    alignItems: 'center',
    justifyContent: 'center',
  },
  checkIcon: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  radioIcon: {
    width: `calc(${themeLayout['--spacing']} * 2)`,
    height: `calc(${themeLayout['--spacing']} * 2)`,
    fill: 'currentColor',
  },
  submenuArrow: {
    marginLeft: 'auto',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
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
