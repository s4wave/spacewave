import { useCallback, useMemo } from 'react'
import { LuBriefcase } from '@s4wave/web/images/icons.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { buildForgeObjectKey } from '../space/create-op-builders.js'

import { useWizardState } from './useWizardState.js'
import { WizardShell } from './WizardShell.js'

import * as stylex from '@stylexjs/stylex'

import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    marginRight: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
  },
})

const stylexStyles = stylex.create({
  rule1: { width: '100%', maxWidth: 'var(--container-sm)' },
})

export const ForgeJobWizardTypeID = 'wizard/forge/job'

// ForgeJobWizardViewer is a custom wizard viewer for creating Forge Jobs.
// Step 0: config editor (cluster + tasks). Step 1: job name. Finalize creates the job.
export function ForgeJobWizardViewer(props: ObjectViewerComponentProps) {
  // Read the current job wizard and its editable configuration.
  const ws = useWizardState(props, 'forge/job')
  const { state, configEditor } = ws

  // Resolve the selected cluster and task definitions from the draft.
  const config = configEditor.value as ForgeJobCreateOp | undefined
  const selectedCluster = config?.clusterKey ?? ''
  const taskDefs = useMemo(() => config?.taskDefs ?? [], [config?.taskDefs])

  // Persist the job draft before advancing to its review step.
  const handleNext = useCallback(async () => {
    const handle = ws.wizardResource.value
    if (!handle) return
    await ws.persistDraftState()
    await handle.updateState({ step: 1 })
  }, [ws])

  // Create the configured job after its required inputs are complete.
  const handleFinalize = useCallback(async () => {
    // Keep only named task definitions before creating the job.
    if (!state || ws.creating || !selectedCluster || !ws.localName.trim())
      return
    const validTasks = taskDefs.flatMap((task) => {
      const name = task.name?.trim()
      return name ? [{ ...task, name }] : []
    })
    if (validTasks.length === 0) return

    // Persist the final draft and create the job with its task definitions.
    ws.setCreating(true)
    try {
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

  // Start job creation from the final confirmation control.
  const handleFinalizeClick = useCallback(() => {
    void handleFinalize()
  }, [handleFinalize])

  // Cancel the current job wizard through its owning handle.
  const handleCancel = useCallback(() => {
    void ws.handleCancel()
  }, [ws])

  // Show wizard initialization until its state is available.
  if (!state) {
    return (
      <div sx={stylexStyles3.rule1}>
        <LoadingCard
          view={{
            state: 'loading',
            title: 'Loading wizard',
            detail: 'Initializing Forge job configuration.',
          }}
          sx={stylexStyles.rule1}
        />
      </div>
    )
  }

  return (
    <WizardShell
      title={
        <>
          <LuBriefcase sx={stylexStyles2.rule1} />
          New Forge Job
        </>
      }
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
      creating={ws.creating}
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
      {(childSx) => (
        <>{(state.step ?? 0) === 0 && configEditor.render(childSx)}</>
      )}
    </WizardShell>
  )
}
