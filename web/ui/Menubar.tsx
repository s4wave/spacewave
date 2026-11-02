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
import * as MenubarPrimitive from '@radix-ui/react-menubar'
import { LuCheck, LuChevronRight, LuCircle } from '@s4wave/web/images/icons.js'

import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'

import { layout as themeLayout } from '../style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  radii as themeRadii,
  colors as themeColors,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const menuStyles = stylex.create({
  root: { display: 'flex', alignItems: 'center' },
  shell: {
    gap: '1px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: 0,
    backgroundColor: '#0000',
    padding: 0,
    '--tw-shadow': '0 0 #0000',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  content: {
    zIndex: 50,
    maxHeight: 'var(--radix-menubar-content-available-height)',
    minWidth: '8rem',
    transformOrigin: 'var(--radix-menubar-content-transform-origin)',
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
  dimmedItem: {
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
    opacity: { default: 0.5, ':is([data-disabled])': 0.5 },
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
      ':is([data-highlighted])': themeColors['--color-accent'],
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
      ':is([data-highlighted])': themeColors['--color-accent-foreground'],
      ':is([data-variant="destructive"])': {
        default: themeColors['--color-destructive'],
        ':focus': themeColors['--color-destructive'],
      },
    },
    pointerEvents: { default: null, ':is([data-disabled])': 'none' },
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
      ':is([data-highlighted])': themeColors['--color-accent'],
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
      ':is([data-highlighted])': themeColors['--color-accent-foreground'],
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
    backgroundColor: {
      default: null,
      ':focus': themeColors['--color-accent'],
      ':is([data-highlighted])': themeColors['--color-accent'],
    },
    color: {
      default: null,
      ':focus': themeColors['--color-accent-foreground'],
      ':is([data-highlighted])': themeColors['--color-accent-foreground'],
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
      ':is([data-highlighted])': themeColors['--color-accent'],
      ':is([data-state="open"])': themeColors['--color-accent'],
    },
    color: {
      default: null,
      ':focus': themeColors['--color-accent-foreground'],
      ':is([data-highlighted])': themeColors['--color-accent-foreground'],
      ':is([data-state="open"])': themeColors['--color-accent-foreground'],
    },
  },
  subContent: {
    zIndex: 50,
    minWidth: '8rem',
    transformOrigin: 'var(--radix-menubar-content-transform-origin)',
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

function Menubar({
  sx,
  variant,
  ...props
}: Omit<
  React.ComponentProps<typeof MenubarPrimitive.Root> & {
    variant?: 'default' | 'shell'
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <MenubarPrimitive.Root
      data-slot="menubar"
      {...stylexProps(
        [menuStyles.root, variant === 'shell' && menuStyles.shell, sx],
        props,
      )}
    />
  )
}

function MenubarMenu({
  ...props
}: React.ComponentProps<typeof MenubarPrimitive.Menu>) {
  return <MenubarPrimitive.Menu data-slot="menubar-menu" {...props} />
}

function MenubarGroup({
  sx,
  ...props
}: Omit<React.ComponentProps<typeof MenubarPrimitive.Group>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <MenubarPrimitive.Group
      data-slot="menubar-group"
      {...stylexProps(sx, props)}
    />
  )
}

function MenubarPortal({
  ...props
}: React.ComponentProps<typeof MenubarPrimitive.Portal>) {
  return <MenubarPrimitive.Portal data-slot="menubar-portal" {...props} />
}

function MenubarTrigger({
  sx,
  ...props
}: Omit<React.ComponentProps<typeof MenubarPrimitive.Trigger>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <MenubarPrimitive.Trigger
      data-slot="menubar-trigger"
      {...stylexProps([sx], props)}
    />
  )
}

function MenubarContent({
  sx,
  align = 'start',
  sideOffset = 4,
  ...props
}: Omit<React.ComponentProps<typeof MenubarPrimitive.Content>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  const environment = useAppEnvironment()
  return (
    <MenubarPortal>
      <MenubarPrimitive.Content
        data-spacewave-app={environment.id || undefined}
        data-slot="menubar-content"
        align={align}
        sideOffset={sideOffset}
        {...stylexProps([menuStyles.content, sx], props)}
      />
    </MenubarPortal>
  )
}

function MenubarItem({
  sx,
  inset,
  variant = 'default',
  ...props
}: Omit<
  React.ComponentProps<typeof MenubarPrimitive.Item> & {
    inset?: boolean
    variant?: 'default' | 'destructive' | 'dimmed'
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <IconStylesProvider
      sx={[iconDefaults.menu, iconDefaults.menuMuted]}
      requiredSx={[iconRequired.menu, iconRequired.menuDestructive]}
    >
      <MenubarPrimitive.Item
        data-slot="menubar-item"
        data-inset={inset}
        data-variant={variant}
        {...stylexProps(
          [
            menuIconMarker,
            menuMutedIconMarker,
            [
              variant === 'dimmed' ? menuStyles.dimmedItem : menuStyles.item,
              sx,
            ],
          ],
          props,
        )}
      />
    </IconStylesProvider>
  )
}

function MenubarCheckboxItem({
  sx,
  children,
  checked,
  ...props
}: Omit<
  React.ComponentProps<typeof MenubarPrimitive.CheckboxItem>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <IconStylesProvider sx={[iconDefaults.menu]} requiredSx={iconRequired.menu}>
      <MenubarPrimitive.CheckboxItem
        data-slot="menubar-checkbox-item"
        checked={checked}
        {...stylexProps([menuIconMarker, [menuStyles.choiceItem, sx]], props)}
      >
        <span sx={indicatorStyles.indicator}>
          <MenubarPrimitive.ItemIndicator>
            <LuCheck sx={indicatorStyles.checkIcon} />
          </MenubarPrimitive.ItemIndicator>
        </span>
        {children}
      </MenubarPrimitive.CheckboxItem>
    </IconStylesProvider>
  )
}

function MenubarRadioGroup({
  sx,
  ...props
}: Omit<
  React.ComponentProps<typeof MenubarPrimitive.RadioGroup>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <MenubarPrimitive.RadioGroup
      data-slot="menubar-radio-group"
      {...stylexProps(sx, props)}
    />
  )
}

function MenubarRadioItem({
  sx,
  children,
  ...props
}: Omit<
  React.ComponentProps<typeof MenubarPrimitive.RadioItem>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <IconStylesProvider sx={[iconDefaults.menu]} requiredSx={iconRequired.menu}>
      <MenubarPrimitive.RadioItem
        data-slot="menubar-radio-item"
        {...stylexProps([menuIconMarker, [menuStyles.choiceItem, sx]], props)}
      >
        <span sx={indicatorStyles.indicator}>
          <MenubarPrimitive.ItemIndicator>
            <LuCircle sx={indicatorStyles.radioIcon} />
          </MenubarPrimitive.ItemIndicator>
        </span>
        {children}
      </MenubarPrimitive.RadioItem>
    </IconStylesProvider>
  )
}

function MenubarLabel({
  sx,
  inset,
  ...props
}: Omit<
  React.ComponentProps<typeof MenubarPrimitive.Label> & {
    inset?: boolean
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <MenubarPrimitive.Label
      data-slot="menubar-label"
      data-inset={inset}
      {...stylexProps([menuStyles.label, sx], props)}
    />
  )
}

function MenubarSeparator({
  sx,
  ...props
}: Omit<
  React.ComponentProps<typeof MenubarPrimitive.Separator>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <MenubarPrimitive.Separator
      data-slot="menubar-separator"
      {...stylexProps([menuStyles.separator, sx], props)}
    />
  )
}

function MenubarShortcut({
  sx,
  ...props
}: Omit<React.ComponentProps<'span'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <span
      data-slot="menubar-shortcut"
      {...stylexProps([menuStyles.shortcut, sx], props)}
    />
  )
}

function MenubarSub({
  ...props
}: React.ComponentProps<typeof MenubarPrimitive.Sub>) {
  return <MenubarPrimitive.Sub data-slot="menubar-sub" {...props} />
}

function MenubarSubTrigger({
  sx,
  inset,
  children,
  ...props
}: Omit<
  React.ComponentProps<typeof MenubarPrimitive.SubTrigger> & {
    inset?: boolean
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <IconStylesProvider
      sx={[iconDefaults.menu, iconDefaults.menuMuted]}
      requiredSx={iconRequired.menu}
    >
      <MenubarPrimitive.SubTrigger
        data-slot="menubar-sub-trigger"
        data-inset={inset}
        {...stylexProps(
          [menuIconMarker, menuMutedIconMarker, [menuStyles.subTrigger, sx]],
          props,
        )}
      >
        {children}
        <LuChevronRight sx={indicatorStyles.submenuArrow} />
      </MenubarPrimitive.SubTrigger>
    </IconStylesProvider>
  )
}

function MenubarSubContent({
  sx,
  ...props
}: Omit<
  React.ComponentProps<typeof MenubarPrimitive.SubContent>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  const environment = useAppEnvironment()
  return (
    <MenubarPrimitive.SubContent
      data-spacewave-app={environment.id || undefined}
      data-slot="menubar-sub-content"
      {...stylexProps([menuStyles.subContent, sx], props)}
    />
  )
}

export {
  Menubar,
  MenubarCheckboxItem,
  MenubarContent,
  MenubarGroup,
  MenubarItem,
  MenubarLabel,
  MenubarMenu,
  MenubarPortal,
  MenubarRadioGroup,
  MenubarRadioItem,
  MenubarSeparator,
  MenubarShortcut,
  MenubarSub,
  MenubarSubContent,
  MenubarSubTrigger,
  MenubarTrigger,
}
