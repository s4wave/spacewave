import { useState, useCallback } from 'react'
import { LuChevronLeft, LuChevronRight, LuCheck } from 'react-icons/lu'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
} from '@s4wave/web/ui/Popover.js'
import {
  listObjectLevel,
  objectLevelPrefix,
  type ObjectTypeMetadataById,
} from '@s4wave/web/space/object-tree.js'

// objectKeySelectorLimit bounds the entries listed for one level.
const objectKeySelectorLimit = 500

export interface ObjectKeySelectorProps {
  world: Resource<IWorldState>
  metadataById?: ObjectTypeMetadataById
  value: string
  onChange: (objectKey: string) => void
  disabled?: boolean
  placeholder?: string
}

// ObjectKeySelector renders a drill-in picker for selecting an object key. It
// lists one level of the World's object tree at a time while open.
export function ObjectKeySelector({
  world,
  metadataById,
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

  const prefix = objectLevelPrefix(path.at(-1) ?? '')
  const level = useWorldQuery(
    world,
    async (state, signal) =>
      open
        ? await listObjectLevel(
            state,
            prefix,
            objectKeySelectorLimit,
            metadataById,
            signal,
          )
        : null,
    [open, prefix, metadataById],
  ).value
  const currentNodes = level?.nodes ?? []

  const handleDrillIn = useCallback((id: string) => {
    setPath((prev) => [...prev, id])
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
        <button type="button"
          disabled={disabled}
          className="border-foreground/8 bg-background-primary text-foreground w-full rounded-lg border px-3 py-1.5 text-left text-xs"
        >
          {displayValue}
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-72 p-0">
        {path.length > 0 && (
          <button type="button"
            onClick={handleBack}
            className="border-foreground/8 flex w-full items-center gap-1 border-b px-3 py-2 text-xs"
          >
            <LuChevronLeft className="size-3.5" />
            {prefix}
          </button>
        )}
        <div className="max-h-[240px] overflow-auto">
          {currentNodes.map((node) => {
            const isFolder = !!node.hasChildren
            const key = node.data?.objectKey ?? node.id
            const isNodeSelected = selected === key
            return (
              <button type="button"
                key={node.id}
                onClick={() =>
                  isFolder ? handleDrillIn(node.id) : handleSelect(key)
                }
                className="hover:bg-foreground/6 flex w-full items-center gap-2 px-3 py-1.5 text-xs"
              >
                <span className="size-4 shrink-0">{node.icon}</span>
                <span className="flex-1 truncate text-left">{node.name}</span>
                {isFolder && (
                  <LuChevronRight className="text-foreground-alt size-3.5" />
                )}
                {!isFolder && isNodeSelected && (
                  <LuCheck className="text-foreground size-3.5" />
                )}
              </button>
            )
          })}
          {level?.more && (
            <div className="text-foreground-alt px-3 py-1.5 text-xs">
              More objects not shown
            </div>
          )}
        </div>
        <div className="border-foreground/8 border-t px-3 py-2">
          <button type="button"
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
