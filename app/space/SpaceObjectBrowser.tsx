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
} from 'react-icons/lu'

import { useOpenCommand } from '@s4wave/web/command/CommandContext.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import {
  buildObjectTree,
  isHiddenSpaceObject,
  type ObjectTreeNode,
} from '@s4wave/web/space/object-tree.js'
import { cn } from '@s4wave/web/style/utils.js'
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
import { Tree } from '@s4wave/web/ui/tree/index.js'
import { applySpaceIndexPath } from './space-settings.js'

export interface SpaceObjectBrowserProps {
  embedded?: boolean
}

interface ContextMenuState {
  position: { x: number; y: number }
  node: TreeNode<ObjectTreeNode>
}

// SpaceObjectBrowser renders the object tree for a space with context menu actions.
export function SpaceObjectBrowser({ embedded }: SpaceObjectBrowserProps) {
  const {
    spaceState,
    navigateToObjects,
    spaceWorld,
    objectKey: currentObjectKey,
  } = SpaceContainerContext.useContext()
  const openCommand = useOpenCommand()
  const indexPath = spaceState.settings?.indexPath ?? ''
  const renameInputId = useId()

  const [menuState, setMenuState] = useState<ContextMenuState | null>(null)
  const [pendingDelete, setPendingDelete] = useState<string | null>(null)
  const [pendingIndex, setPendingIndex] = useState<string | null>(null)
  const [pendingRename, setPendingRename] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [renameSaving, setRenameSaving] = useState(false)

  const objects = spaceState.worldContents?.objects
  const objectCount = useMemo(() => {
    if (!objects) return 0
    return objects.filter(
      (o) => !isHiddenSpaceObject(o.objectKey, o.objectType),
    ).length
  }, [objects])

  const openObject = useCallback(
    (objectKey: string) => {
      navigateToObjects([objectKey])
    },
    [navigateToObjects],
  )

  const setAsIndex = useCallback(
    async (objectKey: string) => {
      await applySpaceIndexPath(spaceWorld, spaceState.settings, objectKey)
      toast.success(`Default object set to ${objectKey}`)
    },
    [spaceWorld, spaceState.settings],
  )

  const handleSetAsIndexClick = useCallback(
    (objectKey: string) => {
      void setAsIndex(objectKey)
    },
    [setAsIndex],
  )

  const treeNodes = useMemo(() => {
    const nodes = buildObjectTree(objects ?? [])
    const addIcons = (list: TreeNode<ObjectTreeNode>[]) => {
      for (const node of list) {
        if (!node.data?.isVirtual) {
          const key = node.data?.objectKey ?? ''
          const isCurrentObject = key === currentObjectKey
          const isIndex = key === indexPath
          node.icons = [
            {
              icon: (
                <LuFolderOpen
                  className={cn('h-3 w-3', isCurrentObject && 'opacity-30')}
                />
              ),
              tooltip: isCurrentObject ? 'Already open' : 'Open',
              onClick: isCurrentObject ? undefined : () => openObject(key),
            },
            {
              icon: (
                <LuHouse className={cn('h-3 w-3', isIndex && 'opacity-30')} />
              ),
              tooltip:
                isIndex ?
                  'This object is the default object already'
                : 'Set as Index',
              onClick: isIndex ? undefined : () => handleSetAsIndexClick(key),
            },
          ]
        }
        if (node.children) addIcons(node.children)
      }
    }
    addIcons(nodes)
    return nodes
  }, [objects, openObject, handleSetAsIndexClick, indexPath, currentObjectKey])

  const handleOpen = useCallback(
    (nodes: TreeNode<ObjectTreeNode>[]) => {
      const data = nodes[0]?.data
      if (!data || data.isVirtual) return
      openObject(data.objectKey)
    },
    [openObject],
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
  const treeCard = (
    <InfoCard>
      <div className="max-h-[300px] overflow-auto">
        <Tree
          nodes={treeNodes}
          onRowDefaultAction={handleOpen}
          onRowContextMenu={handleContextMenu}
          placeholder="No objects"
        />
      </div>
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
        {pendingDelete ?
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
              <LuTrash2 className="h-3.5 w-3.5" />
              Confirm Delete
            </DropdownMenuItem>
          </>
        : pendingIndex ?
          <>
            <DropdownMenuItem disabled>
              Set "{pendingIndex}" as index?
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onSelect={(e) => e.preventDefault()}
              onClick={handleIndexConfirmClick}
            >
              <LuHouse className="h-3.5 w-3.5" />
              Confirm
            </DropdownMenuItem>
          </>
        : pendingRename ?
          <div className="w-80 p-2">
            <div className="border-foreground/6 bg-background-card/30 overflow-hidden rounded-lg border backdrop-blur-sm">
              <div className="border-foreground/8 flex h-9 items-center gap-2 border-b px-3">
                <span className="bg-brand/10 flex h-5 w-5 shrink-0 items-center justify-center rounded-md">
                  <LuPencil className="text-brand h-3 w-3" />
                </span>
                <div className="text-foreground text-xs font-medium tracking-tight select-none">
                  Rename object key
                </div>
              </div>
              <div className="space-y-3 p-3.5">
                <div className="text-foreground-alt/50 flex items-center gap-2 text-[0.6rem]">
                  <span className="border-foreground/6 bg-background/20 min-w-0 truncate rounded-md border px-2 py-1 font-mono">
                    {pendingRename}
                  </span>
                  <LuArrowRight className="h-3 w-3 shrink-0" />
                  <span className="text-foreground-alt/40 shrink-0">
                    new key
                  </span>
                </div>
                <div className="space-y-2">
                  <label
                    htmlFor={renameInputId}
                    className="text-foreground text-xs font-medium select-none"
                  >
                    New object key
                  </label>
                  <Input
                    id={renameInputId}
                    value={renameValue}
                    onChange={(e) => setRenameValue(e.currentTarget.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') void handleRenameConfirm()
                      if (e.key === 'Escape') handleRenameCancel()
                    }}
                    autoFocus
                    disabled={renameSaving}
                    aria-label="New object key"
                    className="border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-9 font-mono text-xs"
                  />
                  <p className="text-foreground-alt/40 text-[0.6rem]">
                    Press Enter to rename, or Escape to cancel.
                  </p>
                </div>
              </div>
              <div className="border-foreground/8 flex items-center justify-end gap-2 border-t px-3 py-2.5">
                <DashboardButton
                  type="button"
                  icon={<LuX className="h-3.5 w-3.5" />}
                  onClick={handleRenameCancel}
                  disabled={renameSaving}
                >
                  Cancel
                </DashboardButton>
                <DashboardButton
                  type="button"
                  icon={<LuCheck className="h-3.5 w-3.5" />}
                  onClick={handleRenameConfirmClick}
                  disabled={!renameValue.trim() || renameSaving}
                  className="border-brand/30 bg-brand/10 text-foreground hover:border-brand/50 hover:bg-brand/15 disabled:cursor-not-allowed disabled:opacity-50"
                >
                  {renameSaving ? 'Saving...' : 'Rename'}
                </DashboardButton>
              </div>
            </div>
          </div>
        : <>
            <DropdownMenuItem onClick={handleMenuOpen}>
              <LuFolderOpen className="h-3.5 w-3.5" />
              Open
            </DropdownMenuItem>
            <DropdownMenuItem onClick={handleSetAsIndex}>
              <LuHouse className="h-3.5 w-3.5" />
              Set as Index
            </DropdownMenuItem>
            <DropdownMenuItem onClick={handleCopyKey}>
              <LuCopy className="h-3.5 w-3.5" />
              Copy Object Key
            </DropdownMenuItem>
            <DropdownMenuItem
              onSelect={(e) => e.preventDefault()}
              onClick={handleRenameClick}
            >
              <LuPencil className="h-3.5 w-3.5" />
              Rename Object Key
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={() => {
                setMenuState(null)
                openCommand('spacewave.create-object')
              }}
            >
              <LuPlus className="h-3.5 w-3.5" />
              New Object
            </DropdownMenuItem>
            <DropdownMenuItem
              variant="destructive"
              onSelect={(e) => e.preventDefault()}
              onClick={handleDeleteClick}
            >
              <LuTrash2 className="h-3.5 w-3.5" />
              Delete
            </DropdownMenuItem>
          </>
        }
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
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-foreground flex items-center gap-1.5 text-xs select-none">
          <LuBox className="h-3.5 w-3.5" />
          Objects
          <span className="text-foreground-alt">({objectCount})</span>
        </h2>
        <DashboardButton
          icon={<LuPlus className="h-3.5 w-3.5" />}
          onClick={handleCreateObject}
        />
      </div>
      {treeCard}
      {menu}
    </section>
  )
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
