import { useCallback } from 'react'

import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { buildObjectKey } from '@s4wave/app/space/create-op-builders.js'
import { WizardShell } from '@s4wave/app/wizard/WizardShell.js'
import { useWizardState } from '@s4wave/app/wizard/useWizardState.js'
import { useWizardFinish } from '@s4wave/app/wizard/wizard-finish.js'
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

// notesSeeders writes each notes type's object and seed content to a World.
const notesSeeders: Record<
  NotesTargetTypeId,
  (world: IWorldState, key: string, name: string, at: Date) => Promise<void>
> = {
  'notes/notebook': (world, key, name, at) =>
    createNotebookClientSide(
      world,
      key,
      buildNotebookUnixfsObjectKey(key),
      name,
      at,
    ),
  'notes/docs': (world, key, name, at) =>
    createDocsClientSide(world, key, name, '', at),
  'notes/blog': (world, key, name, at) =>
    createBlogClientSide(world, key, name, '', '', at),
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

  const finish = useWizardFinish(ws)
  const handleFinalize = useCallback(async () => {
    if (!state) return
    await finish.finish(
      async () => {
        const name = ws.localName || state.name
        if (!name) throw new Error('Enter a name')
        if (!isNotesTargetTypeId(targetTypeId)) {
          throw new Error('Unknown notes type ' + targetTypeId)
        }
        await ws.persistDraftState()
        const targetKey = await buildObjectKey(
          ws.spaceWorld,
          state.targetKeyPrefix ?? '',
          name,
        )
        const seed = notesSeeders[targetTypeId]
        const timestamp = new Date()
        return {
          // The seeders write a UnixFS tree and read back what they wrote, so
          // they run as one writer on the transaction rather than as ops.
          creates: [
            {
              kind: 'write',
              run: (world) => seed(world, targetKey, name, timestamp),
            },
          ],
          target: { openKey: targetKey, indexPath: targetKey, name },
        }
      },
      (target) => {
        toast.success(`Created ${target.name}`)
        ws.navigateToObjects([target.openKey])
      },
    )
  }, [finish, state, targetTypeId, ws])

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
      title={`New ${displayName}`}
      step={state.step ?? 0}
      localName={ws.localName}
      onUpdateName={ws.handleUpdateName}
      onBack={() => void ws.handleBack()}
      onCancel={handleCancel}
      finish={finish.state}
      onFinalize={handleFinalizeClick}
      canFinalize={isNotesTargetTypeId(targetTypeId)}
    />
  )
}

export default NotesWizardViewer
