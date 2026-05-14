import { useCallback } from 'react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { buildObjectKey } from '@s4wave/app/space/create-op-builders.js'
import { WizardShell } from '@s4wave/app/wizard/WizardShell.js'
import { useWizardState } from '@s4wave/app/wizard/useWizardState.js'
import { createBlogClientSide } from './blog-seed.js'
import {
  buildNotebookUnixfsObjectKey,
  createDocsClientSide,
  createNotebookClientSide,
} from './content-seed.js'

export const NotesNotebookWizardTypeID = 'wizard/notes/notebook'
export const NotesDocsWizardTypeID = 'wizard/notes/docs'
export const NotesBlogWizardTypeID = 'wizard/notes/blog'

type NotesTargetTypeId = 'notes/notebook' | 'notes/docs' | 'notes/blog'

const displayNames: Record<NotesTargetTypeId, string> = {
  'notes/notebook': 'Notebook',
  'notes/docs': 'Documentation',
  'notes/blog': 'Blog',
}

function isNotesTargetTypeId(value: string): value is NotesTargetTypeId {
  return (
    value === 'notes/notebook' ||
    value === 'notes/docs' ||
    value === 'notes/blog'
  )
}

function displayNameForTarget(targetTypeId: string): string {
  if (isNotesTargetTypeId(targetTypeId)) {
    return displayNames[targetTypeId]
  }
  return 'Notes'
}

export function NotesWizardViewer(props: ObjectViewerComponentProps) {
  const ws = useWizardState(props, undefined)
  const state = ws.state
  const targetTypeId = state?.targetTypeId ?? ''
  const displayName = displayNameForTarget(targetTypeId)

  const handleFinalize = useCallback(async () => {
    if (!state || ws.creating) return
    const name = ws.localName || state.name
    if (!name || !isNotesTargetTypeId(targetTypeId)) return

    ws.setCreating(true)
    try {
      await ws.persistDraftState()
      const targetKey = buildObjectKey(
        state.targetKeyPrefix ?? '',
        name,
        ws.existingObjectKeys,
      )
      if (targetTypeId === 'notes/notebook') {
        await createNotebookClientSide(
          ws.spaceWorld,
          targetKey,
          buildNotebookUnixfsObjectKey(targetKey),
          name,
          new Date(),
        )
      } else if (targetTypeId === 'notes/docs') {
        await createDocsClientSide(
          ws.spaceWorld,
          targetKey,
          name,
          '',
          new Date(),
        )
      } else {
        await createBlogClientSide(
          ws.spaceWorld,
          targetKey,
          name,
          '',
          '',
          new Date(),
        )
      }
      await ws.spaceWorld.deleteObject(ws.objectKey)
      toast.success(`Created ${name}`)
      ws.navigateToObjects([targetKey])
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to create object',
      )
    } finally {
      ws.setCreating(false)
    }
  }, [state, targetTypeId, ws])

  const handleFinalizeClick = useCallback(() => {
    void handleFinalize()
  }, [handleFinalize])

  const handleCancel = useCallback(() => {
    void ws.handleCancel()
  }, [ws])

  if (!state) {
    return (
      <div className="flex flex-1 items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading wizard',
              detail: 'Preparing the configuration workflow.',
            }}
          />
        </div>
      </div>
    )
  }

  return (
    <WizardShell
      title={<>New {displayName}</>}
      step={state.step ?? 0}
      localName={ws.localName}
      onUpdateName={ws.handleUpdateName}
      onBack={() => void ws.handleBack()}
      onCancel={handleCancel}
      creating={ws.creating}
      onFinalize={handleFinalizeClick}
      canFinalize={isNotesTargetTypeId(targetTypeId)}
    />
  )
}

export default NotesWizardViewer
