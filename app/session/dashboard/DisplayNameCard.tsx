import { useCallback, useId } from 'react'
import { LuPencil, LuUserCog } from 'react-icons/lu'

import type { Session } from '@s4wave/sdk/session/session.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'

import { DisplayNameForm, useDisplayNameEditor } from '../DisplayNameEditor.js'

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
  const setName = useCallback(
    async (displayName: string) => {
      if (!session) throw new Error('The account is not open')
      await session.localProvider.setDisplayName({ displayName })
    },
    [session],
  )
  const editor = useDisplayNameEditor(currentDisplayName, setName)

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
            {editor.state.kind !== 'viewing' ? (
              <DisplayNameForm
                inputId={inputId}
                editor={editor}
                label="Display Name"
              />
            ) : (
              <div className="flex items-center justify-between gap-2">
                <button
                  type="button"
                  className="text-foreground hover:text-foreground-alt min-w-0 flex-1 cursor-text text-left text-xs transition-colors [@media(pointer:coarse)]:min-h-11"
                  onClick={editor.start}
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
