import { useState, useMemo, useCallback } from 'react'
import { LuChevronLeft, LuChevronRight, LuCheck } from 'react-icons/lu'

import {
  Popover,
  PopoverTrigger,
  PopoverContent,
} from '@s4wave/web/ui/Popover.js'
import type { TreeNode } from '@s4wave/web/ui/tree/TreeNode.js'
import type { ObjectTreeNode } from '@s4wave/web/space/object-tree.js'

export interface ObjectKeySelectorProps {
  nodes: TreeNode<ObjectTreeNode>[]
  value: string
  onChange: (objectKey: string) => void
  disabled?: boolean
  placeholder?: string
}

// ObjectKeySelector renders a drill-in picker for selecting an object key from a tree.
export function ObjectKeySelector({
  nodes,
  value,
  onChange,
  disabled,
  placeholder,
}: ObjectKeySelectorProps) {
  const [open, setOpen] = useState(false)
  const [path, setPath] = useState<string[]>([])
  const [selected, setSelected] = useState<string | null>(null)

  const handleOpenChange = useCallback((next: boolean) => {
    setOpen(next)
    if (next) {
      setPath([])
      setSelected(null)
    }
  }, [])

  const currentNodes = useMemo(() => {
    let current = nodes
    for (const segment of path) {
      const found = current.find((n) => n.name === segment)
      if (found?.children) {
        current = found.children
      }
    }
    return current
  }, [nodes, path])

  const handleDrillIn = useCallback((name: string) => {
    setPath((prev) => [...prev, name])
    setSelected(null)
  }, [])

  const handleBack = useCallback(() => {
    setPath((prev) => prev.slice(0, -1))
    setSelected(null)
  }, [])

  const handleSelect = useCallback((key: string) => {
    setSelected(key)
  }, [])

  const handleConfirm = useCallback(() => {
    if (selected) {
      onChange(selected)
      setOpen(false)
    }
  }, [selected, onChange])

  const displayValue = value || placeholder || 'Select...'

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <button
          disabled={disabled}
          className="border-foreground/8 bg-background-primary text-foreground w-full rounded-lg border px-3 py-1.5 text-left text-xs"
        >
          {displayValue}
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-72 p-0">
        {path.length > 0 && (
          <button
            onClick={handleBack}
            className="border-foreground/8 flex w-full items-center gap-1 border-b px-3 py-2 text-xs"
          >
            <LuChevronLeft className="h-3.5 w-3.5" />
            {path.join('/') + '/'}
          </button>
        )}
        <div className="max-h-[240px] overflow-auto">
          {currentNodes.map((node) => {
            const isFolder = !!node.children?.length
            const key = node.data?.objectKey ?? node.id
            const isNodeSelected = selected === key
            return (
              <button
                key={node.id}
                onClick={() =>
                  isFolder ? handleDrillIn(node.name) : handleSelect(key)
                }
                className="hover:bg-foreground/6 flex w-full items-center gap-2 px-3 py-1.5 text-xs"
              >
                <span className="h-4 w-4 shrink-0">{node.icon}</span>
                <span className="flex-1 truncate text-left">{node.name}</span>
                {isFolder && (
                  <LuChevronRight className="text-foreground-alt h-3.5 w-3.5" />
                )}
                {!isFolder && isNodeSelected && (
                  <LuCheck className="text-foreground h-3.5 w-3.5" />
                )}
              </button>
            )
          })}
        </div>
        <div className="border-foreground/8 border-t px-3 py-2">
          <button
            onClick={handleConfirm}
            disabled={!selected}
            className="bg-accent text-accent-foreground w-full rounded-lg px-3 py-1 text-xs disabled:opacity-50"
          >
            Select
          </button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
