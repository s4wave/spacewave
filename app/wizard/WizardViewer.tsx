import { useCallback, useMemo } from 'react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { useConfigEditor } from '@s4wave/web/configtype/useConfigEditor.js'
import {
  lookupCreateOpBuilder,
  buildForgeObjectKey,
  buildObjectKey,
} from '../space/create-op-builders.js'
import { useExperimentalCreatorsEnabled } from '../creator-visibility.js'
import { normalizeObjectWizards } from '../space/object-wizards.js'
import { useObjectWizards } from '../space/useObjectWizards.js'

import { useWizardState } from './useWizardState.js'
import { useWizardFinish, WizardInputError } from './wizard-finish.js'
import { WizardGone } from './WizardGone.js'
import { WizardShell } from './WizardShell.js'
import { WizardLoading } from './WizardLoading.js'

export const WizardTypePrefix = 'wizard/'

const dns1123NamePattern = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/

function validateTargetName(targetTypeId: string | undefined, name: string) {
  if (targetTypeId !== 'forge/cluster' || dns1123NamePattern.test(name)) {
    return ''
  }
  return 'Use 1–63 lowercase letters, numbers, or hyphens; start and end with a letter or number.'
}

export function WizardViewer(props: ObjectViewerComponentProps) {
  const experimentalCreatorsEnabled = useExperimentalCreatorsEnabled()
  const wizards = useObjectWizards().value?.wizards

  // Pass undefined configTypeId: the generic viewer resolves it dynamically.
  const ws = useWizardState(props, undefined)
  const { state } = ws
  const finish = useWizardFinish(ws)

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
    if (!state) return
    await finish.finish(
      async () => {
        // Check and save the draft and choose the key before the write
        // transaction opens, so it holds the writer only for the writes.
        const createOpId = targetWizard?.createOpId
        const builder = createOpId && lookupCreateOpBuilder(createOpId)
        if (!createOpId || !builder) {
          throw new Error('No builder for this object type')
        }
        const name = ws.localName || state.name || ''
        const nameError = name
          ? validateTargetName(state.targetTypeId, name)
          : 'Enter a name'
        if (nameError) throw new WizardInputError(nameError)
        await ws.persistDraftState()
        const key = await (
          state.targetTypeId?.startsWith('forge/')
            ? buildForgeObjectKey
            : buildObjectKey
        )(ws.spaceWorld, state.targetKeyPrefix ?? '', name)
        const opData = builder(key, name, state.configData)
        return {
          creates: [{ kind: 'op', opId: createOpId, opData }],
          target: { openKey: key, indexPath: key, name },
        }
      },
      (target) => {
        toast.success(`Created ${target.name}`)
        ws.navigateToObjects([target.openKey])
      },
    )
  }, [finish, state, targetWizard, ws])

  const handleFinalizeClick = useCallback(() => {
    void handleFinalize()
  }, [handleFinalize])

  const handleCancel = useCallback(() => {
    void ws.handleCancel()
  }, [ws])

  // The gone state shows before the wizard state loads, since a removed
  // wizard may never stream one.
  if (finish.state.status === 'gone') return <WizardGone />

  if (!state) {
    return (
      <WizardLoading
        wizardGone={ws.wizardGone}
        detail="Preparing the configuration workflow."
      />
    )
  }

  const displayName =
    targetWizard?.displayName || state.targetTypeId || 'object'
  const nameError = validateTargetName(state.targetTypeId, ws.localName)

  return (
    <WizardShell
      title={`New ${displayName}`}
      step={state.step ?? 0}
      localName={ws.localName}
      onUpdateName={ws.handleUpdateName}
      nameError={nameError}
      onBack={() => void ws.handleBack()}
      onCancel={handleCancel}
      cancel={ws.cancel}
      finish={finish.state}
      onFinalize={handleFinalizeClick}
      canFinalize={!nameError}
    >
      {configEditor.element}
    </WizardShell>
  )
}
