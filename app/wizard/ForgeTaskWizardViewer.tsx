import { useCallback, useEffect, useRef } from 'react'
import { LuListTodo } from 'react-icons/lu'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { ForgeTaskCreateOp } from '@s4wave/core/forge/task/task.pb.js'
import { buildObjectKey } from '../space/create-op-builders.js'

import { useWizardState } from './useWizardState.js'
import { WizardShell } from './WizardShell.js'

export const ForgeTaskWizardTypeID = 'wizard/forge/task'

// ForgeTaskWizardViewer is a custom wizard viewer for creating Forge Tasks.
// Step 0: select a job. Step 1: task name. Finalize creates the task linked to the job.
export function ForgeTaskWizardViewer(props: ObjectViewerComponentProps) {
  const ws = useWizardState(props, 'forge/task')
  const { state, configEditor } = ws

  const config = configEditor.value as ForgeTaskCreateOp | undefined
  const selectedJob = config?.jobKey ?? ''

  // Auto-advance to step 1 when a job is selected.
  const prevJobRef = useRef(selectedJob)
  useEffect(() => {
    if (selectedJob && selectedJob !== prevJobRef.current) {
      const handle = ws.wizardResource.value
      if (handle) {
        void (async () => {
          await ws.persistDraftState()
          await handle.updateState({ step: 1 })
        })()
      }
    }
    prevJobRef.current = selectedJob
  }, [selectedJob, ws])

  const handleFinalize = useCallback(async () => {
    if (!state || ws.creating || !selectedJob || !ws.localName.trim()) return

    ws.setCreating(true)
    try {
      await ws.persistDraftState()
      const taskKey = buildObjectKey(
        'forge/task/',
        ws.localName,
        ws.existingObjectKeys,
      )
      const opData = ForgeTaskCreateOp.toBinary({
        taskKey,
        name: ws.localName,
        jobKey: selectedJob,
        timestamp: new Date(),
      })
      await ws.spaceWorld.applyWorldOp(
        'spacewave/forge/task/create',
        opData,
        ws.sessionPeerId,
      )
      await ws.spaceWorld.deleteObject(ws.objectKey)
      toast.success(`Created task ${ws.localName}`)
      ws.navigateToObjects([taskKey])
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create task')
    } finally {
      ws.setCreating(false)
    }
  }, [state, ws, selectedJob])

  const handleFinalizeClick = useCallback(() => {
    void handleFinalize()
  }, [handleFinalize])

  const handleCancel = useCallback(() => {
    void ws.handleCancel()
  }, [ws])

  if (!state) {
    return (
      <div className="flex flex-1 items-center justify-center p-4">
        <LoadingCard
          view={{
            state: 'loading',
            title: 'Loading wizard',
            detail: 'Initializing Forge task configuration.',
          }}
          className="w-full max-w-sm"
        />
      </div>
    )
  }

  return (
    <WizardShell
      title={
        <>
          <LuListTodo className="mr-2 h-4 w-4 shrink-0" />
          New Forge Task
        </>
      }
      step={state.step ?? 0}
      totalSteps={2}
      localName={ws.localName}
      onUpdateName={ws.handleUpdateName}
      onBack={() => void ws.handleBack()}
      onCancel={handleCancel}
      nameLabel="Task Name"
      namePlaceholder="Enter task name (DNS label)..."
      nameHelp="Must be a valid DNS label (lowercase alphanumeric and hyphens)."
      nameStep={1}
      creating={ws.creating}
      onFinalize={handleFinalizeClick}
      finalizeStep={1}
    >
      {(state.step ?? 0) === 0 && configEditor.element}
    </WizardShell>
  )
}
