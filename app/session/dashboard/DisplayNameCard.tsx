import { useEffect, useId, useState } from 'react'
import { LuPencil, LuSave, LuUserCog, LuX } from 'react-icons/lu'

import type { Session } from '@s4wave/sdk/session/session.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { Input } from '@s4wave/web/ui/input.js'

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
  session,
  currentDisplayName,
  open,
  onOpenChange,
}: DisplayNameCardProps) {
  const inputId = useId()
  const editor = useDisplayNameEditor(session, currentDisplayName)

  return (
    <CollapsibleSection
      title="Account"
      icon={<LuUserCog className="size-3.5" />}
      open={open}
      onOpenChange={onOpenChange}
    >
      <InfoCard>
        <div className="space-y-2">
          <div>
            <label
              htmlFor={inputId}
              className="text-foreground-alt mb-1 block text-xs select-none"
            >
              Display Name
            </label>
            {editor.editing ? (
              <DisplayNameForm inputId={inputId} editor={editor} />
            ) : (
              <div className="flex items-center justify-between gap-2">
                <button
                  type="button"
                  className="text-foreground hover:text-foreground-alt min-w-0 flex-1 cursor-text text-left text-xs transition-colors"
                  onDoubleClick={editor.start}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      editor.start()
                    }
                  }}
                >
                  {currentDisplayName || 'Unnamed account'}
                </button>
                <DashboardButton
                  icon={<LuPencil className="size-3" />}
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
      <div className="flex items-center gap-2">
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
          icon={<LuSave className="size-3" />}
          onClick={() => void editor.save()}
          disabled={!editor.changed || editor.saving}
        >
          {editor.saving ? 'Saving…' : 'Save'}
        </DashboardButton>
        <DashboardButton
          icon={<LuX className="size-3" />}
          onClick={editor.cancel}
          disabled={editor.saving}
        >
          Cancel
        </DashboardButton>
      </div>
      {editor.error && (
        <div className="text-destructive mt-1 text-xs">{editor.error}</div>
      )}
    </>
  )
}
