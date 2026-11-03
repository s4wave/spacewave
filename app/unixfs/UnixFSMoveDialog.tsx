import { useCallback, useMemo, useRef, useState } from 'react'
import { LuCheck, LuFolder } from '@s4wave/web/images/icons.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'

import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@s4wave/web/ui/command.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import {
  describeUnixFSMoveValidation,
  listUnixFSDirectories,
  type UnixFSMoveItem,
  validateUnixFSMove,
} from './move.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule2: {
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule4: {
    paddingLeft: 'var(--docs-tree-padding-left)',
    display: 'flex',
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: { textOverflow: 'ellipsis', whiteSpace: 'nowrap', overflow: 'hidden' },
  rule6: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule7: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeLayout['--text-metadata'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule8: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule9: {
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule10: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-brand'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
})

const stylexStyles2 = stylex.create({
  rule1: { maxHeight: `calc(${themeLayout['--spacing']} * 72)` },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-file-folder-icon'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-brand'],
  },
})

export interface UnixFSMoveDialogProps {
  rootHandle: FSHandle
  moveItems: UnixFSMoveItem[]
  initialSelectedPath?: string | null
  onOpenChange: (open: boolean) => void
  onConfirm: (destinationPath: string) => Promise<void>
}

function getMoveDialogTitle(moveItems: UnixFSMoveItem[]): string {
  if (moveItems.length === 1) {
    return `Move ${moveItems[0].name}`
  }
  return `Move ${moveItems.length} items`
}

function getMoveDialogDescription(moveItems: UnixFSMoveItem[]): string {
  if (moveItems.length === 1) {
    return 'Choose the destination folder for this item.'
  }
  return 'Choose the destination folder for the selected items.'
}

// UnixFSMoveDialog renders the destination picker for same-UnixFS move actions.
export function UnixFSMoveDialog({
  rootHandle,
  moveItems,
  initialSelectedPath = null,
  onOpenChange,
  onConfirm,
}: UnixFSMoveDialogProps) {
  // Keep the initial destination and submission result for this dialog lifetime.
  const initialSelectedPathRef = useRef(initialSelectedPath)
  const [selectedPath, setSelectedPath] = useState<string | null>(
    initialSelectedPathRef.current,
  )
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  // Load directory choices through the cancellable directory operation.
  const dirs = usePromise(
    useCallback(
      (signal: AbortSignal) => listUnixFSDirectories(rootHandle, signal),
      [rootHandle],
    ),
  )

  // Validate the selected destination before enabling confirmation.
  const validation = useMemo(() => {
    if (!selectedPath) return null
    return validateUnixFSMove(moveItems, selectedPath)
  }, [moveItems, selectedPath])
  const validationMessage = useMemo(
    () =>
      validation && !validation.accepted
        ? describeUnixFSMoveValidation(validation)
        : null,
    [validation],
  )

  // Clear submission feedback when the dialog closes.
  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open) {
        setSubmitError(null)
        setSubmitting(false)
      }
      onOpenChange(open)
    },
    [onOpenChange],
  )

  // Submit the validated destination and retain a retryable failure message.
  const handleConfirm = useCallback(async () => {
    if (!selectedPath || (validation && !validation.accepted)) return
    setSubmitting(true)
    setSubmitError(null)
    try {
      await onConfirm(selectedPath)
      handleOpenChange(false)
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Move failed')
      setSubmitting(false)
    }
  }, [handleOpenChange, onConfirm, selectedPath, validation])

  // Disable confirmation until directory loading and destination validation succeed.
  const confirmDisabled =
    !selectedPath ||
    submitting ||
    dirs.loading ||
    !!dirs.error ||
    (validation ? !validation.accepted : false)

  // Prefer destination validation feedback over the last submission failure.
  const errorMessage = validationMessage ?? submitError

  return (
    <Dialog open onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{getMoveDialogTitle(moveItems)}</DialogTitle>
          <DialogDescription>
            {getMoveDialogDescription(moveItems)}
          </DialogDescription>
        </DialogHeader>

        <div sx={stylexStyles3.rule1}>
          <Command variant="folder">
            <CommandInput placeholder="Search folders…" />
            <CommandList sx={stylexStyles2.rule1}>
              {dirs.loading && (
                <div sx={stylexStyles3.rule2}>
                  <LoadingInline
                    label="Loading folders"
                    tone="muted"
                    size="sm"
                  />
                </div>
              )}
              {dirs.error && (
                <div sx={stylexStyles3.rule3}>
                  Failed to load folders: {dirs.error.message}
                </div>
              )}
              {!dirs.loading && !dirs.error && (
                <>
                  <CommandEmpty variant="folder">
                    No folders found.
                  </CommandEmpty>
                  <CommandGroup>
                    {(dirs.data ?? []).map((dir) => {
                      const isSelected = selectedPath === dir.path
                      return (
                        <CommandItem
                          key={dir.path}
                          value={`${dir.path} ${dir.name}`}
                          onSelect={() => {
                            setSelectedPath(dir.path)
                            setSubmitError(null)
                          }}
                          variant="compact"
                        >
                          <div
                            sx={stylexStyles3.rule4}
                            style={{
                              '--docs-tree-padding-left': `${dir.depth * 12}px`,
                            }}
                          >
                            <LuFolder sx={stylexStyles.rule1} />
                            <span sx={stylexStyles3.rule5}>{dir.name}</span>
                          </div>
                          {isSelected && <LuCheck sx={stylexStyles.rule2} />}
                        </CommandItem>
                      )
                    })}
                  </CommandGroup>
                </>
              )}
            </CommandList>
          </Command>
        </div>

        {selectedPath && (
          <div sx={stylexStyles3.rule6}>
            <span>Moving to</span>
            <code sx={stylexStyles3.rule7}>{selectedPath}</code>
          </div>
        )}

        {errorMessage && <p sx={stylexStyles3.rule8}>{errorMessage}</p>}

        <DialogFooter>
          <button
            type="button"
            sx={stylexStyles3.rule9}
            onClick={() => handleOpenChange(false)}
            disabled={submitting}
          >
            Cancel
          </button>
          <button
            type="button"
            sx={[stylexStyles3.rule10]}
            onClick={() => void handleConfirm()}
            disabled={confirmDisabled}
          >
            {submitting ? 'Moving…' : 'Move'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
