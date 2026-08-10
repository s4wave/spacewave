import { useCallback, useMemo } from 'react'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { SpaceContext } from '@s4wave/web/contexts/contexts.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { useConfigEditor } from '@s4wave/web/configtype/useConfigEditor.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import {
  lookupCreateOpBuilder,
  buildObjectKey,
} from '../space/create-op-builders.js'
import { useExperimentalCreatorsEnabled } from '../creator-visibility.js'
import { normalizeObjectWizards } from '../space/object-wizards.js'
import { useWizardState } from './useWizardState.js'
import { WizardShell } from './WizardShell.js'

export const WizardTypePrefix = 'wizard/'

export function WizardViewer(props: ObjectViewerComponentProps) {
  const spaceResource = SpaceContext.useContext()
  const space = useResourceValue(spaceResource)
  const experimentalCreatorsEnabled = useExperimentalCreatorsEnabled()
  const { data: wizards } = usePromise(
    useCallback((signal) => space?.listWizards(signal), [space]),
  )

  // Pass undefined configTypeId: the generic viewer resolves it dynamically.
  const ws = useWizardState(props, undefined)
  const { state } = ws

  // Resolve configTypeId from the wizard state and use a separate config editor.
  const configEditor = useConfigEditor(
    state?.targetTypeId,
    state?.configData ?? undefined,
    ws.handleConfigDataChange,
  )

  const targetWizard = useMemo(
    () =>
      normalizeObjectWizards(wizards ?? [], experimentalCreatorsEnabled).find(
        (w) => w.typeId === state?.targetTypeId,
      ),
    [experimentalCreatorsEnabled, wizards, state?.targetTypeId],
  )

  const handleFinalize = useCallback(async () => {
    if (!state || !targetWizard?.createOpId || ws.creating) return
    const name = ws.localName || state.name
    const targetKeyPrefix = state.targetKeyPrefix ?? ''
    if (!name) return

    const builder = lookupCreateOpBuilder(targetWizard.createOpId)
    if (!builder) {
      toast.error('No builder for this object type')
      return
    }

    ws.setCreating(true)
    try {
      await ws.persistDraftState()
      const targetKey = buildObjectKey(
        targetKeyPrefix,
        name,
        ws.existingObjectKeys,
      )
      const opData = builder(targetKey, name, state.configData)
      await ws.spaceWorld.applyWorldOp(
        targetWizard.createOpId,
        opData,
        ws.sessionPeerId,
      )
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
  }, [state, targetWizard, ws])

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

  const displayName = targetWizard?.displayName ?? state.targetTypeId

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
    >
      {configEditor.element}
    </WizardShell>
  )
}
