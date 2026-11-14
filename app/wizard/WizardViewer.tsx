import { useCallback, useMemo } from 'react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { useConfigEditor } from '@s4wave/web/configtype/useConfigEditor.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { useObjectQuery } from '@s4wave/web/sync/useObjectQuery.js'
import { useShellTabs, useTabId } from '../ShellTabContext.js'
import {
  lookupCreateOpBuilder,
  buildForgeObjectKey,
  buildObjectKey,
} from '../space/create-op-builders.js'
import { useExperimentalCreatorsEnabled } from '../creator-visibility.js'
import { normalizeObjectWizards } from '../space/object-wizards.js'
import { useObjectWizards } from '../space/useObjectWizards.js'

import { useWizardState } from './useWizardState.js'
import { useWizardFinish } from './wizard-finish.js'
import { WizardShell } from './WizardShell.js'

export const WizardTypePrefix = 'wizard/'

const dns1123NamePattern = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/

function validateTargetName(targetTypeId: string | undefined, name: string) {
  if (targetTypeId !== 'forge/cluster' || dns1123NamePattern.test(name)) {
    return ''
  }
  return 'Use 1–63 lowercase letters, numbers, or hyphens; start and end with a letter or number.'
}

// objectExists projects a present wizard object to true. The watch reports null
// once the object is gone.
const objectExists = () => Promise.resolve(true)

// WizardDeleted tells the person the watched wizard object is gone and offers
// to close the shell tab that showed it.
function WizardDeleted() {
  const tabId = useTabId()
  const { closeShellTab } = useShellTabs()
  const closeTab = useCallback(() => {
    if (tabId) closeShellTab(tabId)
  }, [tabId, closeShellTab])
  return (
    <div className="flex flex-1 items-center justify-center p-6">
      <div className="w-full max-w-sm">
        <LoadingCard
          view={{
            state: 'error',
            title: 'Wizard deleted',
            detail: 'This wizard no longer exists.',
            onCancel: tabId ? closeTab : undefined,
            cancelLabel: 'Close tab',
          }}
        />
      </div>
    </div>
  )
}

export function WizardViewer(props: ObjectViewerComponentProps) {
  const experimentalCreatorsEnabled = useExperimentalCreatorsEnabled()
  const wizards = useObjectWizards().value?.wizards

  // Pass undefined configTypeId: the generic viewer resolves it dynamically.
  const ws = useWizardState(props, undefined)
  const { state } = ws
  const finish = useWizardFinish(ws)
  const finishing =
    finish.state.status === 'finishing' || finish.state.status === 'done'

  // The watched object decides the deleted state: a settled watch that finds
  // no object means the wizard is gone, whatever state was last streamed.
  // Finish deletes the wizard itself before it navigates away, so the watch
  // can report the object gone once the Finish commits; that is not a
  // deletion to show.
  const watched = useObjectQuery(props.worldState, ws.objectKey, objectExists)
  const deleted =
    !finishing && !watched.loading && !watched.error && watched.value === null

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
        if (nameError) throw new Error(nameError)
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

  if (deleted) return <WizardDeleted />

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
      finish={finish.state}
      onFinalize={handleFinalizeClick}
      canFinalize={!nameError}
    >
      {configEditor.element}
    </WizardShell>
  )
}
