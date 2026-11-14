import { useCallback } from 'react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { buildObjectKey } from '@s4wave/app/space/create-op-builders.js'
import { WizardShell } from '@s4wave/app/wizard/WizardShell.js'
import { useWizardState } from '@s4wave/app/wizard/useWizardState.js'
import {
  useWizardFinish,
  WizardInputError,
  type WizardMutation,
} from '@s4wave/app/wizard/wizard-finish.js'
import { buildNotebookUnixfsObjectKey } from './content-seed.js'
import { CREATE_BLOG_OP_ID } from './proto/create-blog.js'
import { CREATE_DOCS_OP_ID } from './proto/create-docs.js'
import { INIT_NOTEBOOK_OP_ID } from './proto/init-notebook.js'
import { CreateBlogOp } from './proto/blog.pb.js'
import { CreateDocumentationOp } from './proto/docs.pb.js'
import { InitNotebookOp } from './proto/notebook.pb.js'
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

// notesCreateOps is the World op that creates each notes type's object and
// its seed content. The notes plugin applies it inside the Finish batch.
const notesCreateOps: Record<
  NotesTargetTypeId,
  (key: string, name: string, at: Date) => WizardMutation
> = {
  'notes/notebook': (key, name, at) => ({
    kind: 'op',
    opId: INIT_NOTEBOOK_OP_ID,
    opData: InitNotebookOp.toBinary({
      notebookObjectKey: key,
      unixfsObjectKey: buildNotebookUnixfsObjectKey(key),
      name,
      timestamp: at,
    }),
  }),
  'notes/docs': (key, name, at) => ({
    kind: 'op',
    opId: CREATE_DOCS_OP_ID,
    opData: CreateDocumentationOp.toBinary({
      objectKey: key,
      name,
      timestamp: at,
    }),
  }),
  'notes/blog': (key, name, at) => ({
    kind: 'op',
    opId: CREATE_BLOG_OP_ID,
    opData: CreateBlogOp.toBinary({ objectKey: key, name, timestamp: at }),
  }),
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
        const targetKey = await buildObjectKey(
          ws.spaceWorld,
          state.targetKeyPrefix ?? '',
          name,
        )
        return {
          creates: [notesCreateOps[targetTypeId](targetKey, name, new Date())],
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
