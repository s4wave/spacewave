import { useCallback, useEffect, useState } from 'react'

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
  existingObjectKeys: string[]
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

  const [localName, setLocalName] = useState('')
  const [nameDirty, setNameDirty] = useState(false)
  const [draftConfigData, setDraftConfigData] = useState<
    Uint8Array | undefined
  >(undefined)
  const [configDirty, setConfigDirty] = useState(false)
  const [creating, setCreating] = useState(false)

  // Sync local name from remote state on first load or external change.
  const remoteName = state?.name ?? ''
  useEffect(() => {
    if (nameDirty) {
      if (localName === remoteName) setNameDirty(false)
      return
    }
    setLocalName(remoteName)
  }, [localName, nameDirty, remoteName])

  const remoteConfigData = state?.configData ?? undefined
  useEffect(() => {
    if (configDirty) {
      if (bytesEqual(draftConfigData, remoteConfigData)) setConfigDirty(false)
      return
    }
    setDraftConfigData(remoteConfigData)
  }, [configDirty, draftConfigData, remoteConfigData])

  const handleConfigDataChange = useCallback(
    (data: Uint8Array) => {
      setDraftConfigData(data)
      setConfigDirty(!bytesEqual(data, remoteConfigData))
    },
    [remoteConfigData],
  )
  const configData = draftConfigData ?? remoteConfigData
  const configEditor = useConfigEditor(
    configTypeId,
    configData,
    handleConfigDataChange,
  )

  const persistDraftState = useCallback(async () => {
    const handle = wizardResource.value
    if (!handle) return
    const configData = draftConfigData ?? new Uint8Array()
    const update: {
      name?: string
      configData?: Uint8Array
    } = {}
    if (nameDirty && localName !== remoteName && localName !== '') {
      update.name = localName
    }
    if (configDirty && !bytesEqual(configData, remoteConfigData)) {
      update.configData = configData
    }
    if (update.name === undefined && update.configData === undefined) return
    await handle.updateState(update)
  }, [
    configDirty,
    draftConfigData,
    localName,
    nameDirty,
    remoteConfigData,
    remoteName,
    wizardResource,
  ])

  const handleUpdateName = useCallback(
    (name: string) => {
      setLocalName(name)
      setNameDirty(name !== remoteName)
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
    existingObjectKeys:
      spaceState.worldContents?.objects?.map((obj) => obj.objectKey ?? '') ??
      [],
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
  return a.every((value, index) => value === b[index])
}
