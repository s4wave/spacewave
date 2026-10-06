import { useCallback, useState } from 'react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { WizardHandle } from '@s4wave/sdk/world/wizard/wizard.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useConfigEditor } from '@s4wave/web/configtype/useConfigEditor.js'
import type { WizardState } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import type { SpaceSettings } from '@s4wave/core/space/world/world.pb.js'

// UseWizardStateResult is the return type of useWizardState.
export interface UseWizardStateResult {
  objectKey: string
  state: WizardState | undefined
  localName: string
  creating: boolean
  setCreating: (v: boolean) => void
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
  handleCancel: () => Promise<void>
}

// useWizardState encapsulates the shared wizard resource access, local name
// sync, cancel, back, and config-data-change logic used by all wizard viewers.
export function useWizardState(
  { objectInfo, worldState }: ObjectViewerComponentProps,
  configTypeId: string | undefined,
): UseWizardStateResult {
  const objectKey = getObjectKey(objectInfo)
  const { spaceState, spaceWorld, navigateToObjects } =
    SpaceContainerContext.useContext()

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
  const [creating, setCreating] = useState(false)
  if (draftName !== null && draftName === remoteName) setDraftName(null)
  if (draftConfig && bytesEqual(draftConfig.data, remoteConfigData)) {
    setDraftConfig(null)
  }
  const localName = draftName ?? remoteName
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

  const handleCancel = useCallback(async () => {
    await spaceWorld.deleteObject(objectKey)
  }, [spaceWorld, objectKey])

  return {
    objectKey,
    state,
    localName,
    creating,
    setCreating,
    sessionPeerId,
    spaceWorld,
    spaceSettings: spaceState.settings,
    navigateToObjects,
    wizardResource,
    configEditor,
    configData,
    persistDraftState,
    handleConfigDataChange,
    handleUpdateName,
    handleBack,
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
