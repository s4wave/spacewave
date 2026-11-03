import { useCallback, useEffect, useRef } from 'react'
import { LuListTodo } from '@s4wave/web/images/icons.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { ForgeTaskCreateOp } from '@s4wave/core/forge/task/task.pb.js'
import { buildForgeObjectKey } from '../space/create-op-builders.js'

import { useWizardState } from './useWizardState.js'
import { WizardShell } from './WizardShell.js'

import * as stylex from '@stylexjs/stylex'

import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    flex: 1,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    marginRight: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: { maxWidth: 'var(--container-sm)', width: '100%' },
})

export const ForgeTaskWizardTypeID = 'wizard/forge/task'

// ForgeTaskWizardViewer is a custom wizard viewer for creating Forge Tasks.
// Step 0: select a job. Step 1: task name. Finalize creates the task linked to the job.
export function ForgeTaskWizardViewer(props: ObjectViewerComponentProps) {
  // Read the current task wizard and its editable configuration.
  const ws = useWizardState(props, 'forge/task')
  const { state, configEditor } = ws

  // Resolve the selected job from the task draft.
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

  // Create the named task only when its job and wizard state are available.
  const handleFinalize = useCallback(async () => {
    if (!state || ws.creating || !selectedJob || !ws.localName.trim()) return

    ws.setCreating(true)
    try {
      await ws.persistDraftState()
      const taskKey = await buildForgeObjectKey(
        ws.spaceWorld,
        'forge/task/',
        ws.localName,
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

  // Start task creation from the final confirmation control.
  const handleFinalizeClick = useCallback(() => {
    void handleFinalize()
  }, [handleFinalize])

  // Cancel the current task wizard through its owning handle.
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
            detail: 'Initializing Forge task configuration.',
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
          <LuListTodo sx={stylexStyles2.rule1} />
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
      {(childSx) => (
        <>{(state.step ?? 0) === 0 && configEditor.render(childSx)}</>
      )}
    </WizardShell>
  )
}
