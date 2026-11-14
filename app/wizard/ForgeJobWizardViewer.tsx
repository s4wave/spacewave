import { useCallback, useMemo } from 'react'
import { LuBriefcase } from 'react-icons/lu'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { buildForgeObjectKey } from '../space/create-op-builders.js'

import { useWizardState } from './useWizardState.js'
import { useWizardFinish, WizardInputError } from './wizard-finish.js'
import { WizardShell } from './WizardShell.js'
import { WizardLoading } from './WizardLoading.js'

export const ForgeJobWizardTypeID = 'wizard/forge/job'

// ForgeJobWizardViewer is a custom wizard viewer for creating Forge Jobs.
// Step 0: config editor (cluster + tasks). Step 1: job name. Finalize creates the job.
export function ForgeJobWizardViewer(props: ObjectViewerComponentProps) {
  const ws = useWizardState(props, 'forge/job')
  const { state, configEditor } = ws
  const finish = useWizardFinish(ws)

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
    if (!state) return
    await finish.finish(
      async () => {
        // Check and save the draft and choose the key before the write
        // transaction opens, so it holds the writer only for the writes.
        const validTasks = taskDefs.flatMap((task) => {
          const name = task.name?.trim()
          return name ? [{ ...task, name }] : []
        })
        if (!selectedCluster || !ws.localName.trim()) {
          throw new WizardInputError('Choose a cluster and name the job')
        }
        if (validTasks.length === 0)
          throw new WizardInputError('Add a named task')
        await ws.persistDraftState()
        const jobKey = await buildForgeObjectKey(
          ws.spaceWorld,
          'forge/job/',
          ws.localName,
        )
        const opData = ForgeJobCreateOp.toBinary({
          jobKey,
          clusterKey: selectedCluster,
          taskDefs: validTasks,
          timestamp: new Date(),
        })
        return {
          creates: [{ kind: 'op', opId: 'spacewave/forge/job/create', opData }],
          target: { openKey: jobKey, indexPath: jobKey, name: ws.localName },
        }
      },
      (target) => {
        toast.success(`Created ${target.name}`)
        ws.navigateToObjects([target.openKey])
      },
    )
  }, [finish, state, ws, selectedCluster, taskDefs])

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
        detail="Initializing Forge job configuration."
      />
    )
  }

  return (
    <WizardShell
      icon={<LuBriefcase />}
      title="New Forge Job"
      step={state.step ?? 0}
      totalSteps={2}
      stepName={(state.step ?? 0) === 0 ? 'Cluster and tasks' : 'Job identity'}
      localName={ws.localName}
      onUpdateName={ws.handleUpdateName}
      onBack={() => void ws.handleBack()}
      onCancel={handleCancel}
      nameLabel="Job Name"
      namePlaceholder="Enter job name..."
      nameHelp="This display name is separate from the stable Job object key."
      nameStep={1}
      finish={finish.state}
      onFinalize={handleFinalizeClick}
      onNext={() => void handleNext()}
      canNext={
        !!selectedCluster && taskDefs.some((task) => !!task.name?.trim())
      }
      canFinalize={
        !!selectedCluster && taskDefs.some((task) => !!task.name?.trim())
      }
      finalizeStep={1}
    >
      {(state.step ?? 0) === 0 && configEditor.element}
    </WizardShell>
  )
}
