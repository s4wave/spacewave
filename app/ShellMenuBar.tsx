import { CommandSurface } from '@s4wave/sdk/command/command.pb.js'
import { useMemo, useCallback } from 'react'
import { LuMenu } from '@s4wave/web/images/icons.js'

import { AppLogo } from '@s4wave/web/images/AppLogo.js'
import {
  Menubar,
  MenubarContent,
  MenubarItem,
  MenubarMenu,
  MenubarSeparator,
  MenubarShortcut,
  MenubarSub,
  MenubarSubContent,
  MenubarSubTrigger,
  MenubarTrigger,
} from '@s4wave/web/ui/Menubar.js'
import {
  useCommands,
  useInvokeCommand,
  useOpenCommand,
} from '@s4wave/web/command/index.js'
import { formatKeybindingHint } from '@s4wave/web/command/CommandPalette.js'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'
import {
  getCommandMenuBinding,
  type KeybindingGraph,
} from '@s4wave/web/command/KeybindingResolver.js'
import { useKeybindingGraph } from '@s4wave/web/command/useKeybindingGraph.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  colors as themeColors,
  motion as themeMotion,
} from '../web/style/tokens.stylex.js'

const stylexStyles5 = stylex.create({
  rule1: {
    gap: '1px',
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    height: '100%',
    paddingLeft: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: themeLayout['--spacing'],
  },
  rule2: {
    alignItems: 'center',
    cursor: 'pointer',
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    marginTop: '-1px',
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule3: { alignItems: 'center', display: 'none', height: '100%' },
  rule4: {
    '--tw-duration': '.2s',
    WebkitUserSelect: 'none',
    gap: '1px',
    overflow: 'hidden',
    alignItems: 'center',
    display: 'flex',
    opacity: { default: null, '@media (max-width: 640px)': 0 },
    transitionDuration: '.2s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
    height: '100%',
    width: { default: null, '@media (max-width: 640px)': 0 },
  },
})

const stylexStyles4 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-menu-button'],
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-pulldown-hover'],
      },
    },
    color: {
      default: themeColors['--color-topbar-button-text'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-topbar-button-text-hi'],
      },
    },
    display: 'flex',
    fontSize: themeLayout['--text-ui'],
    justifyContent: 'center',
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    height: {
      default: `calc(${themeLayout['--spacing']} * 7)`,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    width: {
      default: `calc(${themeLayout['--spacing']} * 7)`,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  rule2: {
    borderRadius: themeRadii['--radius-menu-button'],
    paddingInline: `calc(${themeLayout['--spacing']} * 1.75)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      ':is([data-state="open"])': themeColors['--color-pulldown-hover'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-pulldown-hover'],
      },
    },
    color: {
      default: themeColors['--color-topbar-button-text'],
      ':is([data-state="open"])': themeColors['--color-topbar-button-text-hi'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-topbar-button-text-hi'],
      },
    },
    display: 'flex',
    fontSize: themeLayout['--text-ui'],
    justifyContent: 'center',
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    textShadow: themeLayout['--text-shadow-glow'],
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    whiteSpace: 'nowrap',
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: { height: '100%' },
  rule2: {
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

// MenuNode represents a node in the menu tree.
interface MenuNode {
  label: string
  commandId?: string
  keybindings?: string[]
  hasSubItems?: boolean
  enabled?: boolean
  children: Map<string, MenuNode>
  group: number
  order: number
}

interface MenuSeparatorNode {
  separatorKey: string
}

// topLevelMenus defines the order of top-level menus.
const topLevelMenus = ['File', 'Go', 'Edit', 'View', 'Tools', 'Help']

// buildMenuTree builds a tree of MenuNode from the active commands.
function buildMenuTree(
  commands: CommandState[],
  bindingGraph: KeybindingGraph,
): Map<string, MenuNode> {
  const root = new Map<string, MenuNode>()
  const seen = new Set<string>()

  for (const cmd of commands) {
    const commandId = cmd.command?.commandId
    if (!commandId || seen.has(commandId)) continue
    if (!cmd.active) continue
    seen.add(commandId)
    const menuPath = cmd.command?.menuPath
    if (!menuPath) continue

    const segments = menuPath.split('/')
    if (segments.length < 2) continue

    const topName = segments[0]
    let topNode = root.get(topName)
    if (!topNode) {
      topNode = {
        label: topName,
        children: new Map(),
        group: 0,
        order: 0,
      }
      root.set(topName, topNode)
    }

    // Walk remaining segments to find or create the leaf node.
    let parent = topNode
    for (let i = 1; i < segments.length; i++) {
      const seg = segments[i]
      if (i === segments.length - 1) {
        // Leaf node: the actual command.
        const nodeEnabled = cmd.enabled !== false
        const menuBinding = getCommandMenuBinding(bindingGraph, commandId)
        parent.children.set(seg, {
          label: seg,
          commandId,
          keybindings: menuBinding ? [menuBinding] : [],
          hasSubItems: cmd.command?.hasSubItems,
          enabled: nodeEnabled,
          children: new Map(),
          group: cmd.command?.menuGroup ?? 0,
          order: cmd.command?.menuOrder ?? 0,
        })
      } else {
        // Intermediate node: submenu.
        let sub = parent.children.get(seg)
        if (!sub) {
          sub = {
            label: seg,
            children: new Map(),
            group: 0,
            order: 0,
          }
          parent.children.set(seg, sub)
        }
        parent = sub
      }
    }
  }

  return root
}

// sortedGroupedChildren returns children sorted by group then order,
// with separators between groups indicated by null entries.
function sortedGroupedChildren(
  children: Map<string, MenuNode>,
): (MenuNode | MenuSeparatorNode)[] {
  const items = Array.from(children.values())
  items.sort((a, b) => {
    if (a.group !== b.group) return a.group - b.group
    return a.order - b.order
  })

  const result: (MenuNode | MenuSeparatorNode)[] = []
  let lastGroup = -1
  for (const item of items) {
    if (lastGroup >= 0 && item.group !== lastGroup) {
      result.push({ separatorKey: `sep-${lastGroup}-${item.group}` })
    }
    result.push(item)
    lastGroup = item.group
  }
  return result
}

function isMenuSeparator(
  item: MenuNode | MenuSeparatorNode,
): item is MenuSeparatorNode {
  return 'separatorKey' in item
}

// MenuItemRenderer renders a single menu item or submenu.
function MenuItemRenderer({
  node,
  onSelectCommand,
}: {
  node: MenuNode
  onSelectCommand: (node: MenuNode) => void
}) {
  if (node.children.size > 0 && !node.commandId) {
    const items = sortedGroupedChildren(node.children)
    return (
      <MenubarSub>
        <MenubarSubTrigger>{node.label}</MenubarSubTrigger>
        <MenubarSubContent data-phone-menu>
          {items.map((child) => {
            if (isMenuSeparator(child)) {
              return <MenubarSeparator key={child.separatorKey} />
            }
            return (
              <MenuItemRenderer
                key={child.label}
                node={child}
                onSelectCommand={onSelectCommand}
              />
            )
          })}
        </MenubarSubContent>
      </MenubarSub>
    )
  }

  const disabled = node.enabled === false
  return (
    <MenubarItem
      disabled={disabled}
      onSelect={() => !disabled && node.commandId && onSelectCommand(node)}
      variant={disabled ? 'dimmed' : 'default'}
    >
      {node.label}
      {node.keybindings && node.keybindings.length > 0 && (
        <MenubarShortcut>
          {formatKeybindingHint(node.keybindings)}
        </MenubarShortcut>
      )}
    </MenubarItem>
  )
}

function EmptyMenuItem() {
  return (
    <MenubarItem disabled variant="dimmed">
      No items
    </MenubarItem>
  )
}

/** ShellMenuBar renders command access beside the shell tabs, with a touch menu on phones. */
export function ShellMenuBar() {
  const commands = useCommands()
  const invokeCommand = useInvokeCommand()
  const openCommand = useOpenCommand()
  const bindingGraph = useKeybindingGraph(commands, {
    surface: CommandSurface.WEB,
  })

  const menuTree = useMemo(
    () => buildMenuTree(commands, bindingGraph),
    [commands, bindingGraph],
  )

  const handleSelectCommand = useCallback(
    (node: MenuNode) => {
      if (!node.commandId) return
      if (node.hasSubItems) {
        openCommand(node.commandId)
        return
      }
      invokeCommand(node.commandId)
    },
    [invokeCommand, openCommand],
  )

  const handleLogoClick = useCallback(() => {
    openCommand('spacewave.view.palette')
  }, [openCommand])

  return (
    <div className={'shell-menu-bar'} sx={stylexStyles5.rule1}>
      <button
        className={'shell-menu-palette'}
        type="button"
        aria-label="Open command palette"
        sx={stylexStyles5.rule2}
        onClick={handleLogoClick}
        title="Open command palette"
      >
        <AppLogo sx={stylexStyles3.rule1} />
      </button>
      <div className={'shell-menu-mobile'} sx={stylexStyles5.rule3}>
        <Menubar variant="shell" sx={stylexStyles2.rule1}>
          <MenubarMenu>
            <MenubarTrigger asChild>
              <button
                type="button"
                aria-label="Open app menu"
                sx={stylexStyles4.rule1}
              >
                <LuMenu sx={stylexStyles.rule1} />
              </button>
            </MenubarTrigger>
            <MenubarContent align="start" data-phone-menu>
              {topLevelMenus.map((name) => {
                const node = menuTree.get(name)
                const items = node ? sortedGroupedChildren(node.children) : []
                return (
                  <MenubarSub key={name}>
                    <MenubarSubTrigger sx={stylexStyles2.rule2}>
                      {name}
                    </MenubarSubTrigger>
                    <MenubarSubContent data-phone-menu>
                      {items.length ? (
                        items.map((item) =>
                          isMenuSeparator(item) ? (
                            <MenubarSeparator key={item.separatorKey} />
                          ) : (
                            <MenuItemRenderer
                              key={item.commandId ?? item.label}
                              node={item}
                              onSelectCommand={handleSelectCommand}
                            />
                          ),
                        )
                      ) : (
                        <EmptyMenuItem />
                      )}
                    </MenubarSubContent>
                  </MenubarSub>
                )
              })}
            </MenubarContent>
          </MenubarMenu>
        </Menubar>
      </div>
      <div className={'shell-menu-collapsible'} sx={[stylexStyles5.rule4]}>
        <Menubar variant="shell" sx={stylexStyles2.rule1}>
          {topLevelMenus.map((name) => {
            const node = menuTree.get(name)
            const items = node ? sortedGroupedChildren(node.children) : []
            return (
              <MenubarMenu key={name}>
                <MenubarTrigger asChild>
                  <button type="button" sx={stylexStyles4.rule2}>
                    {name}
                  </button>
                </MenubarTrigger>
                <MenubarContent align="start">
                  {items.length ? (
                    items.map((item) => {
                      if (isMenuSeparator(item)) {
                        return <MenubarSeparator key={item.separatorKey} />
                      }
                      return (
                        <MenuItemRenderer
                          key={item.commandId ?? item.label}
                          node={item}
                          onSelectCommand={handleSelectCommand}
                        />
                      )
                    })
                  ) : (
                    <EmptyMenuItem />
                  )}
                </MenubarContent>
              </MenubarMenu>
            )
          })}
        </Menubar>
      </div>
    </div>
  )
}
