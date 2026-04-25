import { useCallback, useMemo } from 'react'
import { LuBriefcase } from 'react-icons/lu'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { buildObjectKey } from '../space/create-op-builders.js'

import { useWizardState } from './useWizardState.js'
import { WizardShell } from './WizardShell.js'

export const ForgeJobWizardTypeID = 'wizard/forge/job'

// ForgeJobWizardViewer is a custom wizard viewer for creating Forge Jobs.
// Step 0: config editor (cluster + tasks). Step 1: job name. Finalize creates the job.
export function ForgeJobWizardViewer(props: ObjectViewerComponentProps) {
  const ws = useWizardState(props, 'forge/job')
  const { state, configEditor } = ws

  const config = configEditor.value as ForgeJobCreateOp | undefined
  const selectedCluster = config?.clusterKey ?? ''
  const taskDefs = useMemo(() => config?.taskDefs ?? [], [config?.taskDefs])

  const handleNext = useCallback(async () => {
    const handle = ws.wizardResource.value
    if (!handle) return
    await ws.persistDraftState()
    await handle.updateState({ step: 1 })
  }, [ws])

  const handleFinalize = useCallback(async () => {
    if (!state || ws.creating || !selectedCluster || !ws.localName.trim())
      return
    const validTasks = taskDefs.filter((td) => td.name)
    if (validTasks.length === 0) return

    ws.setCreating(true)
    try {
      await ws.persistDraftState()
      const jobKey = buildObjectKey(
        'forge/job/',
        ws.localName,
        ws.existingObjectKeys,
      )
      const opData = ForgeJobCreateOp.toBinary({
        jobKey,
        clusterKey: selectedCluster,
        taskDefs: validTasks,
        timestamp: new Date(),
      })
      await ws.spaceWorld.applyWorldOp(
        'spacewave/forge/job/create',
        opData,
        ws.sessionPeerId,
      )
      await ws.spaceWorld.deleteObject(ws.objectKey)
      toast.success(`Created ${ws.localName}`)
      ws.navigateToObjects([jobKey])
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create job')
    } finally {
      ws.setCreating(false)
    }
  }, [state, ws, selectedCluster, taskDefs])

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
            detail: 'Initializing Forge job configuration.',
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
          <LuBriefcase className="mr-2 h-4 w-4 shrink-0" />
          New Forge Job
        </>
      }
      step={state.step ?? 0}
      totalSteps={2}
      localName={ws.localName}
      onUpdateName={ws.handleUpdateName}
      onBack={() => void ws.handleBack()}
      onCancel={handleCancel}
      nameLabel="Job Name"
      namePlaceholder="Enter job name..."
      nameStep={1}
      creating={ws.creating}
      onFinalize={handleFinalizeClick}
      onNext={() => void handleNext()}
      canNext={!!selectedCluster && taskDefs.some((td) => td.name)}
      finalizeStep={1}
    >
      {(state.step ?? 0) === 0 && configEditor.element}
    </WizardShell>
  )
}
