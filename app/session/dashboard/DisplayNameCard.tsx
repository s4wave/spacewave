import { controlTargetStyles } from '@s4wave/web/style/ControlStylesProvider.js'
import { useEffect, useId, useState } from 'react'
import { LuPencil, LuSave, LuUserCog, LuX } from '@s4wave/web/images/icons.js'

import type { Session } from '@s4wave/sdk/session/session.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { Input } from '@s4wave/web/ui/input.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    marginBottom: themeLayout['--spacing'],
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule3: {
    flex: 1,
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    cursor: 'text',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    minWidth: 0,
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule5: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: themeLayout['--spacing'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

// useDisplayNameEditor edits the local account's display name. The draft
// resets whenever the saved name changes.
function useDisplayNameEditor(
  session: Session | null | undefined,
  currentDisplayName: string,
) {
  const [displayName, setDisplayName] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)
  const changed = displayName.trim() !== currentDisplayName

  // Reset the editor only while this display name belongs to the mounted panel.
  useEffect(() => {
    let active = true
    queueMicrotask(() => {
      if (!active) return
      setDisplayName(currentDisplayName)
      setError(null)
      setEditing(false)
    })
    return () => {
      active = false
    }
  }, [currentDisplayName])

  const save = async () => {
    if (!session || saving || !changed) return
    setError(null)
    setSaving(true)
    try {
      await session.localProvider.setDisplayName({
        displayName: displayName.trim(),
      })
      setEditing(false)
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to update account name',
      )
    } finally {
      setSaving(false)
    }
  }

  const start = () => {
    if (saving) return
    setDisplayName(currentDisplayName)
    setError(null)
    setEditing(true)
  }

  const cancel = () => {
    setDisplayName(currentDisplayName)
    setError(null)
    setEditing(false)
  }

  const edit = (value: string) => {
    setDisplayName(value)
    setError(null)
  }

  return {
    displayName,
    error,
    editing,
    saving,
    changed,
    save,
    start,
    cancel,
    edit,
  }
}

export interface DisplayNameCardProps {
  session: Session | null | undefined
  currentDisplayName: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

// DisplayNameCard renders the local account section with an editable display
// name.
export function DisplayNameCard({
  sx,
  session,
  currentDisplayName,
  open,
  onOpenChange,
}: DisplayNameCardProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const inputId = useId()
  const editor = useDisplayNameEditor(session, currentDisplayName)

  return (
    <CollapsibleSection
      sx={sx}
      title="Account"
      icon={<LuUserCog sx={stylexStyles.rule1} />}
      open={open}
      onOpenChange={onOpenChange}
    >
      <InfoCard>
        <div>
          <div sx={nativeChildSpacing.s2}>
            <label htmlFor={inputId} sx={stylexStyles2.rule1}>
              Display Name
            </label>
            {editor.editing ? (
              <DisplayNameForm inputId={inputId} editor={editor} />
            ) : (
              <div sx={stylexStyles2.rule2}>
                <button
                  type="button"
                  sx={[stylexStyles2.rule3, controlTargetStyles.minimum]}
                  onClick={editor.start}
                >
                  {currentDisplayName || 'Unnamed account'}
                </button>
                <DashboardButton
                  icon={<LuPencil sx={stylexStyles.rule2} />}
                  onClick={editor.start}
                >
                  Edit
                </DashboardButton>
              </div>
            )}
          </div>
        </div>
      </InfoCard>
    </CollapsibleSection>
  )
}

// DisplayNameForm renders the display name input with its save and cancel
// actions.
function DisplayNameForm({
  inputId,
  editor,
}: {
  inputId: string
  editor: ReturnType<typeof useDisplayNameEditor>
}) {
  return (
    <>
      <div sx={stylexStyles2.rule4}>
        <Input
          id={inputId}
          value={editor.displayName}
          onChange={(e) => editor.edit(e.target.value)}
          onKeyDown={(e) => {
            if (e.nativeEvent.isComposing) return
            if (e.key === 'Enter') {
              e.preventDefault()
              void editor.save()
            }
            if (e.key === 'Escape') {
              e.preventDefault()
              editor.cancel()
            }
          }}
          placeholder="Name this local account"
          aria-label="Display Name"
          variant="displayName"
        />
        <DashboardButton
          icon={<LuSave sx={stylexStyles.rule2} />}
          onClick={() => void editor.save()}
          disabled={!editor.changed || editor.saving}
        >
          {editor.saving ? 'Saving…' : 'Save'}
        </DashboardButton>
        <DashboardButton
          icon={<LuX sx={stylexStyles.rule2} />}
          onClick={editor.cancel}
          disabled={editor.saving}
        >
          Cancel
        </DashboardButton>
      </div>
      {editor.error && <div sx={stylexStyles2.rule5}>{editor.error}</div>}
    </>
  )
}

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
