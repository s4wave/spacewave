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

import * as stylex from '@stylexjs/stylex'
import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: '100%',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: themeColors['--color-background-primary'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule2: { width: `calc(${themeLayout['--spacing']} * 72)`, padding: 0 },
  rule3: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule5: { maxHeight: '240px', overflow: 'auto' },
  rule6: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      },
    },
  },
  rule7: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
  },
  rule8: {
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textAlign: 'left',
  },
  rule9: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule10: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-foreground'],
  },
  rule11: {
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule12: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule13: {
    width: '100%',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: themeColors['--color-accent'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-accent-foreground'],
    opacity: { default: null, ':disabled': 0.5 },
  },
})

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
  // Keep the open selector, drilled path and pending selection together.
  const [open, setOpen] = useState(false)
  const [path, setPath] = useState<string[]>([])
  const [selected, setSelected] = useState<string | null>(null)

  // Start each opened selector at its root without a pending selection.
  const handleOpenChange = useCallback((next: boolean) => {
    setOpen(next)
    if (next) {
      setPath([])
      setSelected(null)
    }
  }, [])

  // Watch the current object level only while the selector is open.
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

  // Enter a child level and clear the selection from the previous level.
  const handleDrillIn = useCallback((id: string) => {
    setPath((prev) => [...prev, id])
    setSelected(null)
  }, [])

  // Return to the parent level without carrying a child selection.
  const handleBack = useCallback(() => {
    setPath((prev) => prev.slice(0, -1))
    setSelected(null)
  }, [])

  // Keep selection pending until the person confirms the object.
  const handleSelect = useCallback((key: string) => {
    setSelected(key)
  }, [])

  // Publish the confirmed key and close the selector.
  const handleConfirm = useCallback(() => {
    if (selected) {
      onChange(selected)
      setOpen(false)
    }
  }, [selected, onChange])

  // Use the current key or the configured empty prompt for the trigger.
  const displayValue = value || placeholder || 'Select...'

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <button type="button" disabled={disabled} sx={stylexStyles.rule1}>
          {displayValue}
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" sx={stylexStyles.rule2}>
        {path.length > 0 && (
          <button type="button" onClick={handleBack} sx={stylexStyles.rule3}>
            <LuChevronLeft {...stylex.props(stylexStyles.rule4)} />
            {prefix}
          </button>
        )}
        <div sx={stylexStyles.rule5}>
          {currentNodes.map((node) => {
            const isFolder = !!node.hasChildren
            const key = node.data?.objectKey ?? node.id
            const isNodeSelected = selected === key
            return (
              <button
                type="button"
                key={node.id}
                onClick={() =>
                  isFolder ? handleDrillIn(node.id) : handleSelect(key)
                }
                sx={stylexStyles.rule6}
              >
                <span sx={stylexStyles.rule7}>{node.icon}</span>
                <span sx={stylexStyles.rule8}>{node.name}</span>
                {isFolder && (
                  <LuChevronRight {...stylex.props(stylexStyles.rule9)} />
                )}
                {!isFolder && isNodeSelected && (
                  <LuCheck {...stylex.props(stylexStyles.rule10)} />
                )}
              </button>
            )
          })}
          {level?.more && (
            <div sx={stylexStyles.rule11}>More objects not shown</div>
          )}
        </div>
        <div sx={stylexStyles.rule12}>
          <button
            type="button"
            onClick={handleConfirm}
            disabled={!selected}
            sx={stylexStyles.rule13}
          >
            Select
          </button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
