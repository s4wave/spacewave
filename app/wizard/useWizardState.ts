import { useCallback, useState } from 'react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { WizardHandle } from '@s4wave/sdk/world/wizard/wizard.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useConfigEditor } from '@s4wave/web/configtype/useConfigEditor.js'
import { useObjectQuery } from '@s4wave/web/sync/useObjectQuery.js'
import type { WizardState } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import type { SpaceSettings } from '@s4wave/core/space/world/world.pb.js'

import { useShellTabsIfPresent, useTabId } from '../ShellTabContext.js'
import { cancelWizard, WizardGoneError } from './wizard-finish.js'

// WizardCancelState is the Cancel control's state, from WizardCancel.tla.
// cancelling deletes the wizard; cancelled means this press deleted it and
// the view is leaving; gone means another writer removed the wizard first,
// so the view shows the gone screen; failed keeps the wizard and says why,
// and a new press tries again.
export type WizardCancelState =
  | { status: 'idle' }
  | { status: 'cancelling' }
  | { status: 'cancelled' }
  | { status: 'gone' }
  | { status: 'failed'; error: string }

// UseWizardStateResult is the return type of useWizardState.
export interface UseWizardStateResult {
  objectKey: string
  state: WizardState | undefined
  // wizardGone is true once a settled watch on the wizard object finds it
  // removed, by this view's own Finish or by another writer, but not by this
  // view's own Cancel, which leaves the view itself; or once a Cancel was
  // refused because another writer removed it first.
  wizardGone: boolean
  localName: string
  sessionPeerId: string
  spaceWorld: ReturnType<typeof SpaceContainerContext.useContext>['spaceWorld']
  spaceSettings: SpaceSettings | undefined
  navigateToObjects: ReturnType<
    typeof SpaceContainerContext.useContext
  >['navigateToObjects']
  wizardResource: ReturnType<typeof useAccessTypedHandle<WizardHandle>>
  configEditor: ReturnType<typeof useConfigEditor>
  configData: Uint8Array | undefined
  persistDraftState: () => Promise<void>
  handleConfigDataChange: (data: Uint8Array) => void
  handleUpdateName: (name: string) => void
  handleBack: () => Promise<void>
  // cancel is the Cancel control's state, which WizardShell shows.
  cancel: WizardCancelState
  // handleCancel deletes the wizard in one write transaction. A Space index
  // that names the wizard moves to indexPath, or is cleared when indexPath is
  // omitted. Once the delete commits it leaves the view as a Finish does: it
  // closes the shell tab, or opens the Space root when there is no tab, and
  // the unmount releases the wizard resource and stops its work. It never
  // rejects: a wizard another writer removed first ends in cancel's gone
  // state, and any other failure in its failed state.
  handleCancel: (indexPath?: string) => Promise<void>
}

// objectExists projects a present wizard object to true.
const objectExists = () => Promise.resolve(true)

// useWizardState encapsulates the shared wizard resource access, local name
// sync, cancel, back, and config-data-change logic used by all wizard viewers.
export function useWizardState(
  { objectInfo, worldState }: ObjectViewerComponentProps,
  configTypeId: string | undefined,
): UseWizardStateResult {
  const objectKey = getObjectKey(objectInfo)
  const { spaceState, spaceWorld, navigateToObjects, navigateToRoot } =
    SpaceContainerContext.useContext()
  const tabId = useTabId()
  const shellTabs = useShellTabsIfPresent()

  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const { peerId: sessionPeerId } = useSessionInfo(session)

  const wizardResource = useAccessTypedHandle(
    worldState,
    objectKey,
    WizardHandle,
  )
  const wizardState = useStreamingResource(
    wizardResource,
    (handle, signal) => handle.watchState(signal),
    [],
  )
  const state: WizardState | undefined = wizardState.value ?? undefined

  // The drafts hold edits the remote state has not caught up to. A draft ends
  // as soon as the remote value matches it, so later remote changes show.
  const remoteName = state?.name ?? ''
  const remoteConfigData = state?.configData ?? undefined
  const [draftName, setDraftName] = useState<string | null>(null)
  const [draftConfig, setDraftConfig] = useState<{
    data: Uint8Array
  } | null>(null)
  if (draftName !== null && draftName === remoteName) setDraftName(null)
  if (draftConfig && bytesEqual(draftConfig.data, remoteConfigData)) {
    setDraftConfig(null)
  }
  const localName = draftName ?? remoteName
  useObjectName(remoteName)
  const configData = draftConfig ? draftConfig.data : remoteConfigData

  const handleConfigDataChange = useCallback(
    (data: Uint8Array) => {
      setDraftConfig(bytesEqual(data, remoteConfigData) ? null : { data })
    },
    [remoteConfigData],
  )
  const configEditor = useConfigEditor(
    configTypeId,
    configData,
    handleConfigDataChange,
  )

  const persistDraftState = useCallback(async () => {
    const handle = wizardResource.value
    if (!handle) return
    const update: {
      name?: string
      configData?: Uint8Array
    } = {}
    if (draftName !== null && draftName !== remoteName && draftName !== '') {
      update.name = draftName
    }
    if (draftConfig && !bytesEqual(draftConfig.data, remoteConfigData)) {
      update.configData = draftConfig.data
    }
    if (update.name === undefined && update.configData === undefined) return
    await handle.updateState(update)
  }, [draftConfig, draftName, remoteConfigData, remoteName, wizardResource])

  const handleUpdateName = useCallback(
    (name: string) => {
      setDraftName(name === remoteName ? null : name)
    },
    [remoteName],
  )

  const handleBack = useCallback(async () => {
    const handle = wizardResource.value
    if (!handle || !state) return
    await persistDraftState()
    const step = state.step ?? 0
    if (step > 0) void handle.updateState({ step: step - 1 })
  }, [persistDraftState, wizardResource, state])

  const spaceSettings = spaceState.settings
  const [cancel, setCancel] = useState<WizardCancelState>({ status: 'idle' })
  const closeShellTab = shellTabs?.closeShellTab
  const handleCancel = useCallback(
    async (indexPath = '') => {
      setCancel({ status: 'cancelling' })
      try {
        await cancelWizard({ objectKey, spaceWorld, sessionPeerId }, indexPath)
      } catch (err) {
        if (err instanceof WizardGoneError) {
          setCancel({ status: 'gone' })
          return
        }
        const error = err instanceof Error ? err.message : String(err)
        setCancel({ status: 'failed', error })
        return
      }
      setCancel({ status: 'cancelled' })
      if (tabId && closeShellTab) {
        closeShellTab(tabId)
        return
      }
      navigateToRoot()
    },
    [
      closeShellTab,
      navigateToRoot,
      objectKey,
      sessionPeerId,
      spaceWorld,
      tabId,
    ],
  )

  // The watch reports null once the wizard object is gone. This view's own
  // Cancel, running or landed, never shows as a removal elsewhere.
  const watched = useObjectQuery(worldState, objectKey, objectExists)
  const cancelling =
    cancel.status === 'cancelling' || cancel.status === 'cancelled'
  const wizardGone =
    cancel.status === 'gone' ||
    (!cancelling &&
      !watched.loading &&
      !watched.error &&
      watched.value === null)

  return {
    objectKey,
    state,
    wizardGone,
    localName,
    sessionPeerId,
    spaceWorld,
    spaceSettings,
    navigateToObjects,
    wizardResource,
    configEditor,
    configData,
    persistDraftState,
    handleConfigDataChange,
    handleUpdateName,
    handleBack,
    cancel,
    handleCancel,
  }
}

function bytesEqual(a: Uint8Array | undefined, b: Uint8Array | undefined) {
  if (a === b) return true
  if (!a || !b) return !a?.length && !b?.length
  if (a.length !== b.length) return false
  for (const [i, value] of a.entries()) {
    if (value !== b[i]) return false
  }
  return true
}
