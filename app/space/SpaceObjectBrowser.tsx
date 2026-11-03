import { treeRowActionIconStyles } from '@s4wave/web/ui/tree/styles.stylex.js'
import { useMemo, useState, useCallback, useId } from 'react'
import {
  LuArrowRight,
  LuBox,
  LuCheck,
  LuCopy,
  LuFolderOpen,
  LuHouse,
  LuPencil,
  LuPlus,
  LuTrash2,
  LuX,
} from '@s4wave/web/images/icons.js'

import { useOpenCommand } from '@s4wave/web/command/CommandContext.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import type { ObjectTreeNode } from '@s4wave/web/space/object-tree.js'
import { useObjectTree } from '@s4wave/web/space/useObjectTree.js'
import { RootContext } from '@s4wave/web/contexts/contexts.js'
import { useObjectTypeMetadata } from '@s4wave/web/hooks/useObjectTypeMetadata.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'
import { DropdownMenuGhostAnchor } from '@s4wave/web/ui/DropdownMenuGhostAnchor.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { Input } from '@s4wave/web/ui/input.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import type { TreeNode } from '@s4wave/web/ui/tree/TreeNode.js'
import { Tree, useTreeState } from '@s4wave/web/ui/tree/index.js'
import { applySpaceIndexPath } from './space-settings.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import { colors as themeColors } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: { padding: `calc(${themeLayout['--spacing']} * 3.5)` },
})

const stylexStyles3 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 80)`,
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule3: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule4: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-tight)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule7: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule8: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    flexShrink: 0,
  },
  rule9: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule10: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule11: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'flex-end',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
  },
  rule12: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
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
})

const stylexStyles2 = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const stylexStyles = stylex.create({
  rule1: { maxHeight: `calc(${themeLayout['--spacing']} * 75)` },
})

export interface SpaceObjectBrowserProps {
  embedded?: boolean
}

interface ContextMenuState {
  position: { x: number; y: number }
  node: TreeNode<ObjectTreeNode>
}

// useSpaceObjectBrowserController owns tree projection and object mutations.
function useSpaceObjectBrowserController() {
  const {
    spaceState,
    navigateToObjects,
    spaceWorld,
    spaceWorldResource,
    canDeleteObjects,
    objectKey: currentObjectKey,
  } = SpaceContainerContext.useContext()
  const rootResource = RootContext.useContext()
  const objectTypeMetadataById = useObjectTypeMetadata(
    rootResource,
    spaceState.engineId,
  )
  const openCommand = useOpenCommand()
  const indexPath = spaceState.settings?.indexPath ?? ''
  const renameInputId = useId()

  const [menuState, setMenuState] = useState<ContextMenuState | null>(null)
  const [pendingDelete, setPendingDelete] = useState<string | null>(null)
  const [pendingIndex, setPendingIndex] = useState<string | null>(null)
  const [pendingRename, setPendingRename] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [renameSaving, setRenameSaving] = useState(false)

  const treeState = useTreeState(null, 'tree')
  const tree = useObjectTree(
    spaceWorldResource,
    treeState[0].expandedIds,
    objectTypeMetadataById,
  )
  const { loadMore } = tree

  const openObject = useCallback(
    (objectKey: string) => {
      navigateToObjects([objectKey])
    },
    [navigateToObjects],
  )

  const setAsIndex = useCallback(
    async (objectKey: string) => {
      await applySpaceIndexPath(spaceWorld, objectKey)
      toast.success(`Default object set to ${objectKey}`)
    },
    [spaceWorld],
  )

  const handleSetAsIndexClick = useCallback(
    (objectKey: string) => {
      void setAsIndex(objectKey)
    },
    [setAsIndex],
  )

  const treeNodes = useMemo(() => {
    const withIcons = (
      list: TreeNode<ObjectTreeNode>[],
    ): TreeNode<ObjectTreeNode>[] =>
      list.map((node) => {
        const children = node.children && withIcons(node.children)
        if (node.data?.isVirtual) return { ...node, children }
        const key = node.data?.objectKey ?? ''
        const isCurrentObject = key === currentObjectKey
        const isIndex = key === indexPath
        return {
          ...node,
          children,
          icons: [
            {
              icon: (
                <LuFolderOpen
                  sx={[
                    treeRowActionIconStyles.icon,
                    isCurrentObject && treeActionStyles.dimmed,
                  ]}
                />
              ),
              tooltip: isCurrentObject ? 'Already open' : 'Open',
              onClick: isCurrentObject ? undefined : () => openObject(key),
            },
            {
              icon: (
                <LuHouse
                  sx={[
                    treeRowActionIconStyles.icon,
                    isIndex && treeActionStyles.dimmed,
                  ]}
                />
              ),
              tooltip: isIndex
                ? 'This object is the default object already'
                : 'Set as Index',
              onClick: isIndex ? undefined : () => handleSetAsIndexClick(key),
            },
          ],
        }
      })
    return withIcons(tree.nodes ?? [])
  }, [
    tree.nodes,
    openObject,
    handleSetAsIndexClick,
    indexPath,
    currentObjectKey,
  ])

  const handleOpen = useCallback(
    (nodes: TreeNode<ObjectTreeNode>[]) => {
      const data = nodes[0]?.data
      if (data?.morePrefix !== undefined) {
        loadMore(data.morePrefix)
        return
      }
      if (!data || data.isVirtual) return
      openObject(data.objectKey)
    },
    [loadMore, openObject],
  )

  const handleContextMenu = useCallback(
    (node: TreeNode<ObjectTreeNode>, event: React.MouseEvent) => {
      if (node.data?.isVirtual) return
      setMenuState({
        position: { x: event.clientX, y: event.clientY },
        node,
      })
    },
    [],
  )

  const handleOpenChange = useCallback((open: boolean) => {
    if (!open) {
      setMenuState(null)
      setPendingDelete(null)
      setPendingIndex(null)
      setPendingRename(null)
      setRenameValue('')
      setRenameSaving(false)
    }
  }, [])

  const handleMenuOpen = useCallback(() => {
    const data = menuState?.node.data
    if (!data) return
    openObject(data.objectKey)
    setMenuState(null)
  }, [menuState, openObject])

  const handleSetAsIndex = useCallback(() => {
    const data = menuState?.node.data
    if (!data) return
    setPendingIndex(data.objectKey)
  }, [menuState])

  const handleIndexConfirm = useCallback(async () => {
    if (!pendingIndex) return
    await setAsIndex(pendingIndex)
    setPendingIndex(null)
    setMenuState(null)
  }, [pendingIndex, setAsIndex])

  const handleIndexConfirmClick = useCallback(() => {
    void handleIndexConfirm()
  }, [handleIndexConfirm])

  const handleCopyKey = useCallback(() => {
    const data = menuState?.node.data
    if (!data) return
    void navigator.clipboard.writeText(data.objectKey)
    setMenuState(null)
  }, [menuState])

  const handleRenameClick = useCallback(() => {
    const data = menuState?.node.data
    if (!data) return
    setPendingRename(data.objectKey)
    setRenameValue(data.objectKey)
  }, [menuState])

  const handleRenameCancel = useCallback(() => {
    setPendingRename(null)
    setRenameValue('')
  }, [])

  const handleRenameConfirm = useCallback(async () => {
    const oldKey = pendingRename
    const newKey = renameValue.trim()
    if (!oldKey || !newKey || renameSaving) return
    if (oldKey === newKey) {
      setPendingRename(null)
      setRenameValue('')
      setMenuState(null)
      return
    }

    setRenameSaving(true)
    try {
      const obj = await spaceWorld.renameObject(oldKey, newKey, {
        descendants: true,
      })
      obj.release()
      const nextIndexPath = rewriteLocalObjectReference(
        indexPath,
        oldKey,
        newKey,
      )
      if (nextIndexPath && nextIndexPath !== indexPath) {
        await setAsIndex(nextIndexPath)
      }
      const nextCurrentObjectKey = rewriteLocalObjectReference(
        currentObjectKey,
        oldKey,
        newKey,
      )
      if (nextCurrentObjectKey && nextCurrentObjectKey !== currentObjectKey) {
        openObject(nextCurrentObjectKey)
      }
      setPendingRename(null)
      setRenameValue('')
      setMenuState(null)
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to rename object key',
      )
    } finally {
      setRenameSaving(false)
    }
  }, [
    currentObjectKey,
    indexPath,
    openObject,
    pendingRename,
    renameSaving,
    renameValue,
    setAsIndex,
    spaceWorld,
  ])

  const handleRenameConfirmClick = useCallback(() => {
    void handleRenameConfirm()
  }, [handleRenameConfirm])

  const handleRenameInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  const handleDeleteClick = useCallback(() => {
    const data = menuState?.node.data
    if (!data) return
    setPendingDelete(data.objectKey)
  }, [menuState])

  const handleDeleteConfirm = useCallback(async () => {
    if (!pendingDelete) return
    await spaceWorld.deleteObject(pendingDelete)
    setPendingDelete(null)
    setMenuState(null)
  }, [pendingDelete, spaceWorld])

  const handleDeleteConfirmClick = useCallback(() => {
    void handleDeleteConfirm()
  }, [handleDeleteConfirm])

  const handleCreateObject = useCallback(() => {
    openCommand('spacewave.create-object')
  }, [openCommand])
  return {
    canDeleteObjects,
    handleContextMenu,
    handleCopyKey,
    handleCreateObject,
    handleDeleteClick,
    handleDeleteConfirmClick,
    handleIndexConfirmClick,
    handleMenuOpen,
    handleOpen,
    handleOpenChange,
    handleRenameCancel,
    handleRenameClick,
    handleRenameConfirm,
    handleRenameConfirmClick,
    handleRenameInputRef,
    handleSetAsIndex,
    menuState,
    openCommand,
    pendingDelete,
    pendingIndex,
    pendingRename,
    renameInputId,
    renameSaving,
    renameValue,
    setMenuState,
    setRenameValue,
    treeNodes,
    treeState,
  }
}

type SpaceObjectBrowserController = ReturnType<
  typeof useSpaceObjectBrowserController
>

// SpaceObjectBrowserView renders the tree and its context-menu states.
function SpaceObjectBrowserView({
  controller,
  embedded,
}: {
  controller: SpaceObjectBrowserController
  embedded: boolean | undefined
}) {
  const {
    canDeleteObjects,
    handleContextMenu,
    handleCopyKey,
    handleCreateObject,
    handleDeleteClick,
    handleDeleteConfirmClick,
    handleIndexConfirmClick,
    handleMenuOpen,
    handleOpen,
    handleOpenChange,
    handleRenameCancel,
    handleRenameClick,
    handleRenameConfirm,
    handleRenameConfirmClick,
    handleRenameInputRef,
    handleSetAsIndex,
    menuState,
    openCommand,
    pendingDelete,
    pendingIndex,
    pendingRename,
    renameInputId,
    renameSaving,
    renameValue,
    setMenuState,
    setRenameValue,
    treeNodes,
    treeState,
  } = controller

  const treeCard = (
    <InfoCard>
      <Tree
        nodes={treeNodes}
        onRowDefaultAction={handleOpen}
        onRowContextMenu={handleContextMenu}
        state={treeState}
        placeholder="No objects"
        sx={stylexStyles.rule1}
      />
    </InfoCard>
  )
  const menu = (
    <DropdownMenu open={menuState !== null} onOpenChange={handleOpenChange}>
      <DropdownMenuTrigger asChild>
        <DropdownMenuGhostAnchor
          x={menuState?.position.x ?? 0}
          y={menuState?.position.y ?? 0}
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" side="bottom">
        {pendingDelete ? (
          <>
            <DropdownMenuItem disabled>
              Delete "{pendingDelete}"?
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              variant="destructive"
              onSelect={(e) => e.preventDefault()}
              onClick={handleDeleteConfirmClick}
            >
              <LuTrash2 sx={stylexStyles2.rule1} />
              Confirm Delete
            </DropdownMenuItem>
          </>
        ) : pendingIndex ? (
          <>
            <DropdownMenuItem disabled>
              Set "{pendingIndex}" as index?
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onSelect={(e) => e.preventDefault()}
              onClick={handleIndexConfirmClick}
            >
              <LuHouse sx={stylexStyles2.rule1} />
              Confirm
            </DropdownMenuItem>
          </>
        ) : pendingRename ? (
          <div sx={stylexStyles3.rule1}>
            <div sx={stylexStyles3.rule2}>
              <div sx={stylexStyles3.rule3}>
                <span sx={stylexStyles3.rule4}>
                  <LuPencil sx={stylexStyles2.rule2} />
                </span>
                <div sx={stylexStyles3.rule5}>Rename object key</div>
              </div>
              <div sx={stylexStyles4.rule1}>
                <div sx={[nativeChildSpacingNext.s3, stylexStyles3.rule6]}>
                  <span sx={stylexStyles3.rule7}>{pendingRename}</span>
                  <LuArrowRight sx={stylexStyles2.rule3} />
                  <span sx={stylexStyles3.rule8}>new key</span>
                </div>
                <div sx={nativeChildSpacingNext.s3}>
                  <label
                    htmlFor={renameInputId}
                    sx={[nativeChildSpacing.s2, stylexStyles3.rule9]}
                  >
                    New object key
                  </label>
                  <Input
                    sx={nativeChildSpacing.s2}
                    ref={handleRenameInputRef}
                    id={renameInputId}
                    value={renameValue}
                    onChange={(e) => setRenameValue(e.currentTarget.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') void handleRenameConfirm()
                      if (e.key === 'Escape') handleRenameCancel()
                    }}
                    disabled={renameSaving}
                    aria-label="New object key"
                    variant="configMono"
                  />
                  <p sx={[nativeChildSpacing.s2, stylexStyles3.rule10]}>
                    Press Enter to rename, or Escape to cancel.
                  </p>
                </div>
              </div>
              <div sx={stylexStyles3.rule11}>
                <DashboardButton
                  type="button"
                  icon={<LuX sx={stylexStyles2.rule1} />}
                  onClick={handleRenameCancel}
                  disabled={renameSaving}
                >
                  Cancel
                </DashboardButton>
                <DashboardButton
                  type="button"
                  icon={<LuCheck sx={stylexStyles2.rule1} />}
                  onClick={handleRenameConfirmClick}
                  disabled={!renameValue.trim() || renameSaving}
                  variant="primary"
                >
                  {renameSaving ? 'Saving…' : 'Rename'}
                </DashboardButton>
              </div>
            </div>
          </div>
        ) : (
          <>
            <DropdownMenuItem onClick={handleMenuOpen}>
              <LuFolderOpen sx={stylexStyles2.rule1} />
              Open
            </DropdownMenuItem>
            <DropdownMenuItem onClick={handleSetAsIndex}>
              <LuHouse sx={stylexStyles2.rule1} />
              Set as Index
            </DropdownMenuItem>
            <DropdownMenuItem onClick={handleCopyKey}>
              <LuCopy sx={stylexStyles2.rule1} />
              Copy Object Key
            </DropdownMenuItem>
            <DropdownMenuItem
              onSelect={(e) => e.preventDefault()}
              onClick={handleRenameClick}
            >
              <LuPencil sx={stylexStyles2.rule1} />
              Rename Object Key
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={() => {
                setMenuState(null)
                openCommand('spacewave.create-object')
              }}
            >
              <LuPlus sx={stylexStyles2.rule1} />
              New Object
            </DropdownMenuItem>
            {canDeleteObjects && (
              <DropdownMenuItem
                variant="destructive"
                onSelect={(e) => e.preventDefault()}
                onClick={handleDeleteClick}
              >
                <LuTrash2 sx={stylexStyles2.rule1} />
                Delete
              </DropdownMenuItem>
            )}
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )

  if (embedded) {
    return (
      <>
        {treeCard}
        {menu}
      </>
    )
  }

  return (
    <section>
      <div sx={stylexStyles3.rule12}>
        <h2 sx={stylexStyles3.rule13}>
          <LuBox sx={stylexStyles2.rule1} />
          Objects
        </h2>
        <DashboardButton
          icon={<LuPlus sx={stylexStyles2.rule1} />}
          onClick={handleCreateObject}
        />
      </div>
      {treeCard}
      {menu}
    </section>
  )
}

// SpaceObjectBrowser renders the object tree for a Space with context-menu
// actions.
export function SpaceObjectBrowser({ embedded }: SpaceObjectBrowserProps) {
  const controller = useSpaceObjectBrowserController()
  return <SpaceObjectBrowserView controller={controller} embedded={embedded} />
}

function rewriteLocalObjectReference(
  key: string | undefined,
  oldKey: string,
  newKey: string,
): string | undefined {
  if (!key) return key
  if (key === oldKey) return newKey
  const prefix = oldKey + '/'
  if (!key.startsWith(prefix)) return key
  return newKey + key.slice(oldKey.length)
}

const treeActionStyles = stylex.create({ dimmed: { opacity: 0.3 } })

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
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
})
