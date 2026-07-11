import { useMemo, useCallback } from 'react'

import { cn } from '@s4wave/web/style/utils.js'
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
  getCommandDisplayBindings,
  type KeybindingGraph,
} from '@s4wave/web/command/KeybindingResolver.js'
import { useKeybindingGraph } from '@s4wave/web/command/useKeybindingGraph.js'

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
const topLevelMenus = ['File', 'Edit', 'View', 'Tools', 'Help']

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
        parent.children.set(seg, {
          label: seg,
          commandId,
          keybindings: getCommandDisplayBindings(
            bindingGraph,
            commandId,
            'combo',
          ),
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
        <MenubarSubContent>
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
      className={cn(disabled && 'opacity-50')}
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
    <MenubarItem disabled className="opacity-50">
      No items
    </MenubarItem>
  )
}

// ShellMenuBar renders the application menu bar with logo and menu items.
// This is displayed to the left of the FlexLayout tabs.
// On narrow screens, the menu items collapse and only the logo dropdown remains.
export function ShellMenuBar() {
  const commands = useCommands()
  const invokeCommand = useInvokeCommand()
  const openCommand = useOpenCommand()
  const bindingGraph = useKeybindingGraph(commands)

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
    <div className="flex h-full shrink-0 items-center gap-px pr-1 pl-1.5">
      <button
        aria-label="Open command palette"
        className="-mt-px flex cursor-pointer items-center justify-center"
        onClick={handleLogoClick}
        title="Open command palette"
      >
        <AppLogo className="size-[28px]" />
      </button>
      <div
        className={cn(
          'shell-menu-collapsible flex h-full items-center gap-px overflow-hidden transition-all duration-200 select-none',
          'narrow:w-0 narrow:opacity-0',
        )}
      >
        <Menubar className="h-full gap-px border-0 bg-transparent p-0 shadow-none">
          {topLevelMenus.map((name) => {
            const node = menuTree.get(name)
            const items = node ? sortedGroupedChildren(node.children) : []
            return (
              <MenubarMenu key={name}>
                <MenubarTrigger asChild>
                  <button className="rounded-menu-button text-topbar-button-text hover:text-topbar-button-text-hi hover:bg-pulldown-hover data-[state=open]:text-topbar-button-text-hi data-[state=open]:bg-pulldown-hover text-topbar-menu text-shadow-glow flex h-5 items-center justify-center px-[7px] whitespace-nowrap transition-colors">
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
