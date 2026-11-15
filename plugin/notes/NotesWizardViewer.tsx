import { useCallback } from 'react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { buildObjectKey } from '@s4wave/app/space/create-op-builders.js'
import { WizardShell } from '@s4wave/app/wizard/WizardShell.js'
import { useWizardState } from '@s4wave/app/wizard/useWizardState.js'
import {
  useWizardFinish,
  WizardInputError,
} from '@s4wave/app/wizard/wizard-finish.js'
import type { WorldStage } from '@s4wave/sdk/world/stage.js'
import { buildNotebookUnixfsObjectKey } from './content-seed.js'
import {
  stageBlogCreate,
  stageDocsCreate,
  stageNotebookCreate,
  type StagedNotesCreate,
} from './staged-seed.js'
import { WizardLoading } from '@s4wave/app/wizard/WizardLoading.js'

export const NotesNotebookWizardTypeID = 'wizard/notes/notebook'
export const NotesDocsWizardTypeID = 'wizard/notes/docs'
export const NotesBlogWizardTypeID = 'wizard/notes/blog'

type NotesTargetTypeId = 'notes/notebook' | 'notes/docs' | 'notes/blog'

const displayNames: Record<NotesTargetTypeId, string> = {
  'notes/notebook': 'Notebook',
  'notes/docs': 'Documentation',
  'notes/blog': 'Blog',
}

// notesCreateOps builds each notes type's object and seed content in a World
// stage and returns the World op that adopts them. The notes plugin applies
// the op inside the Finish batch, while the plan holds the stage.
const notesCreateOps: Record<
  NotesTargetTypeId,
  (
    stage: WorldStage,
    key: string,
    name: string,
    at: Date,
  ) => Promise<StagedNotesCreate>
> = {
  'notes/notebook': (stage, key, name) =>
    stageNotebookCreate(stage, key, buildNotebookUnixfsObjectKey(key), name),
  'notes/docs': (stage, key, name, at) => stageDocsCreate(stage, key, name, at),
  'notes/blog': (stage, key, name, at) => stageBlogCreate(stage, key, name, at),
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
        if (!name) throw new WizardInputError('Enter a name')
        if (!isNotesTargetTypeId(targetTypeId)) {
          throw new Error('Unknown notes type ' + targetTypeId)
        }
        await ws.persistDraftState()
        const [key, staged] = await Promise.allSettled([
          buildObjectKey(ws.spaceWorld, state.targetKeyPrefix ?? '', name),
          ws.spaceWorld.stageWorldState(),
        ])
        if (staged.status === 'rejected') throw staged.reason
        // The plan holds the stage until the Finish list returns; a build
        // that fails releases it here.
        const stage = staged.value
        try {
          if (key.status === 'rejected') throw key.reason
          const targetKey = key.value
          const op = await notesCreateOps[targetTypeId](
            stage,
            targetKey,
            name,
            new Date(),
          )
          return {
            creates: [{ kind: 'op', opId: op.opTypeId, opData: op.opData }],
            target: { openKey: targetKey, indexPath: targetKey, name },
            hold: stage,
          }
        } catch (err) {
          stage.release()
          throw err
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
      <WizardLoading
        wizardGone={ws.wizardGone}
        detail="Preparing the configuration workflow."
      />
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
      cancel={ws.cancel}
      finish={finish.state}
      onFinalize={handleFinalizeClick}
      canFinalize={isNotesTargetTypeId(targetTypeId)}
    />
  )
}

export default NotesWizardViewer
