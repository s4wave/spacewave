/* eslint-disable react-doctor/async-await-in-loop, react-doctor/no-giant-component */
import { useCallback, use, useEffect, useMemo, useRef, useState } from 'react'
import {
  LuCloud,
  LuCpu,
  LuHardDrive,
  LuMonitor,
  LuRefreshCcw,
} from 'react-icons/lu'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import {
  useResource,
  useResourceValue,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { SessionIndexContext } from '@s4wave/web/contexts/contexts.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'

import { listObjectsWithType } from '@s4wave/sdk/world/types/types.js'
import { keyToIRI, iriToKey } from '@s4wave/sdk/world/graph-utils.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import type { Cdn } from '@s4wave/sdk/cdn/cdn.js'
import { CopyV86ImageToSpaceStage } from '@s4wave/sdk/cdn/cdn-resource.pb.js'
import type { Space } from '@s4wave/sdk/space/space.js'
import { CreateVmV86Op, V86Image, VmV86 } from '@s4wave/sdk/vm/v86.pb.js'
import {
  V86WizardConfig,
  V86WizardConfig_Source,
} from '@s4wave/sdk/vm/v86-wizard.pb.js'
import { CREATE_VM_V86_OP_ID } from '@s4wave/sdk/vm/create-vm-v86.js'
import { V86ImageTypeID } from '@s4wave/sdk/vm/v86image.js'
import type { Root } from '@s4wave/sdk/root'
import { buildObjectKey } from '../space/create-op-builders.js'
import { markQuickstartStartupBoundary } from '../quickstart/startup-boundary.js'
import {
  compareV86ImageNewestFirst,
  DEFAULT_V86_MEMORY_MB,
  DEFAULT_V86_VGA_MEMORY_MB,
  isDefaultV86Image,
  seedV86WizardConfig,
  V86_USER_IMAGE_OBJECT_KEY,
} from '../vm/v86-wizard-config.js'

import {
  type VmCreationProgress,
  VmCreationProgressScreen,
} from './VmCreationProgressScreen.js'
import { WizardShell } from './WizardShell.js'
import { useWizardState } from './useWizardState.js'

// VmV86WizardTypeID is the wizard block type id for v86 VM creation wizards.
export const VmV86WizardTypeID = 'wizard/vm/v86'

// VmV86TypeID mirrors sdk/vm/v86.go VmV86TypeID; keep these aligned.
const VmV86TypeID = 'vm/v86'

// V86_IMAGE_PRED mirrors sdk/vm/v86.go PredV86Image; keep aligned.
const V86_IMAGE_PRED = '<v86/image>'

const MEMORY_OPTIONS: readonly number[] = [64, 128, 256, 512, 1024]

interface InSpaceV86ImageEntry {
  objectKey: string
  image: V86Image
}

interface ExistingVmInfo {
  objectKey: string
  name: string
  imageKey: string
  createdAt: Date | undefined
}

interface VmCreationRequest {
  copyFromCdn: boolean
  cdnId: string
  cdnSourceObjectKey: string
  imageObjectKey: string
  sessionIndex: number
  spaceId: string
  vmKey: string
  vmName: string
  memoryMb: number
  vgaMemoryMb: number
  networking: boolean
  sessionPeerId: string
  wizardObjectKey: string
  root?: Root
}

interface CdnV86ImageEntry {
  objectKey: string
  image: V86Image
  metadataError?: string
}

interface CdnImageSpaceHandle {
  cdn: Cdn
  space: Space
  [Symbol.dispose](): void
}

async function mountCdnImageSpace(
  root: Root,
  cdnId: string,
  signal: AbortSignal,
): Promise<CdnImageSpaceHandle> {
  const { cdn } = await root.getCdn(cdnId, signal)
  let space: Space | undefined
  try {
    space = await cdn.mountCdnSpace(signal)
    return {
      cdn,
      space,
      [Symbol.dispose]() {
        space?.[Symbol.dispose]()
        cdn[Symbol.dispose]()
      },
    }
  } catch (err) {
    space?.[Symbol.dispose]()
    cdn[Symbol.dispose]()
    throw err
  }
}

export async function loadCdnV86ImagesFromSpace(
  space: Space,
  signal: AbortSignal,
): Promise<CdnV86ImageEntry[]> {
  const world = await space.accessWorldState(false, signal)
  const keys = await listObjectsWithType(world, V86ImageTypeID, signal)
  const out: CdnV86ImageEntry[] = []
  for (const key of keys) {
    using obj = await world.getObject(key, signal)
    if (!obj) continue
    using cursor = await obj.accessWorldState(undefined, signal)
    const resp = await cursor.unmarshal({ blockType: V86ImageTypeID }, signal)
    if (!resp.found || !resp.data?.length) continue
    try {
      out.push({ objectKey: key, image: V86Image.fromBinary(resp.data) })
    } catch (err) {
      out.push({
        objectKey: key,
        image: {
          name: key,
          platform: 'v86',
          description: 'Metadata could not be decoded.',
          tags: [],
        },
        metadataError:
          err instanceof Error ? err.message : 'metadata decode failed',
      })
    }
  }
  out.sort(compareV86ImageNewestFirst)
  return out
}

async function loadCdnV86ImageFromSpace(
  space: Space,
  objectKey: string,
  signal: AbortSignal,
): Promise<CdnV86ImageEntry | undefined> {
  if (!objectKey) return undefined
  const world = await space.accessWorldState(false, signal)
  using obj = await world.getObject(objectKey, signal)
  if (!obj) return undefined
  using cursor = await obj.accessWorldState(undefined, signal)
  const resp = await cursor.unmarshal({ blockType: V86ImageTypeID }, signal)
  if (!resp.found || !resp.data?.length) return undefined
  try {
    return { objectKey, image: V86Image.fromBinary(resp.data) }
  } catch (err) {
    return {
      objectKey,
      image: {
        name: objectKey,
        platform: 'v86',
        description: 'Metadata could not be decoded.',
        tags: [],
      },
      metadataError:
        err instanceof Error ? err.message : 'metadata decode failed',
    }
  }
}

async function loadCdnV86Images(
  root: Root,
  cdnId: string,
  signal: AbortSignal,
): Promise<CdnV86ImageEntry[]> {
  using handle = await mountCdnImageSpace(root, cdnId, signal)
  return loadCdnV86ImagesFromSpace(handle.space, signal)
}

async function loadCdnV86Image(
  root: Root,
  cdnId: string,
  objectKey: string,
  signal: AbortSignal,
): Promise<CdnV86ImageEntry | undefined> {
  using handle = await mountCdnImageSpace(root, cdnId, signal)
  return loadCdnV86ImageFromSpace(handle.space, objectKey, signal)
}

async function discoverDefaultCdnV86Image(
  root: Root,
  cdnId: string,
  signal: AbortSignal,
): Promise<CdnV86ImageEntry | undefined> {
  const entries = await loadCdnV86Images(root, cdnId, signal)
  return entries.find((entry) => isDefaultV86Image(entry.image))
}

async function lookupImageEdge(
  ws: EngineWorldState,
  vmKey: string,
  signal: AbortSignal,
): Promise<string> {
  const resp = await ws.lookupGraphQuads(
    keyToIRI(vmKey),
    V86_IMAGE_PRED,
    undefined,
    undefined,
    1,
    signal,
  )
  const target = resp.quads?.[0]?.obj
  if (!target) return ''
  return iriToKey(target)
}

async function loadInSpaceV86Images(
  ws: EngineWorldState,
  signal: AbortSignal,
): Promise<InSpaceV86ImageEntry[]> {
  const keys = await listObjectsWithType(ws, V86ImageTypeID, signal)
  const out: InSpaceV86ImageEntry[] = []
  for (const key of keys) {
    using obj = await ws.getObject(key, signal)
    if (!obj) continue
    using cursor = await obj.accessWorldState(undefined, signal)
    const resp = await cursor.unmarshal({ blockType: V86ImageTypeID }, signal)
    if (!resp.found || !resp.data?.length) continue
    try {
      out.push({ objectKey: key, image: V86Image.fromBinary(resp.data) })
    } catch {
      /* skip corrupt */
    }
  }
  out.sort(compareV86ImageNewestFirst)
  return out
}

async function loadExistingVms(
  ws: EngineWorldState,
  signal: AbortSignal,
): Promise<ExistingVmInfo[]> {
  const keys = await listObjectsWithType(ws, VmV86TypeID, signal)
  const out: ExistingVmInfo[] = []
  for (const key of keys) {
    using obj = await ws.getObject(key, signal)
    if (!obj) continue
    using cursor = await obj.accessWorldState(undefined, signal)
    const resp = await cursor.unmarshal({ blockType: VmV86TypeID }, signal)
    if (!resp.found || !resp.data?.length) continue
    try {
      const vm = VmV86.fromBinary(resp.data)
      const imageKey = await lookupImageEdge(ws, key, signal)
      out.push({
        objectKey: key,
        name: vm.name || key,
        imageKey,
        createdAt: vm.createdAt,
      })
    } catch {
      /* skip corrupt */
    }
  }
  out.sort((a, b) => {
    const ta = a.createdAt?.getTime() ?? 0
    const tb = b.createdAt?.getTime() ?? 0
    return tb - ta
  })
  return out
}

function decodeConfig(configData: Uint8Array | undefined): V86WizardConfig {
  if (!configData || configData.length === 0) {
    return V86WizardConfig.create({})
  }
  try {
    return V86WizardConfig.fromBinary(configData)
  } catch {
    return V86WizardConfig.create({})
  }
}

function formatImageLabel(img: V86Image): string {
  const name = img.name || img.distro || 'V86Image'
  if (img.version) return `${name} (${img.version})`
  return name
}

export interface V86CatalogErrorCopy {
  title: string
  detail: string
  unpublished: boolean
}

export function getV86CatalogErrorCopy(error: unknown): V86CatalogErrorCopy {
  const message =
    error instanceof Error
      ? error.message
      : typeof error === 'string'
        ? error
        : ''
  const unpublished =
    /published head|not published|no published|empty catalog/i.test(message)
  return unpublished
    ? {
        title: 'No VM images are published yet',
        detail: 'This image catalog has no published images to copy.',
        unpublished: true,
      }
    : {
        title: 'Image catalog unavailable',
        detail: 'The VM image catalog could not be loaded. Try again.',
        unpublished: false,
      }
}

async function* runVmCreation(
  world: EngineWorldState,
  request: VmCreationRequest,
  signal: AbortSignal,
): AsyncGenerator<VmCreationProgress> {
  if (request.copyFromCdn) {
    const root = request.root
    if (!root) throw new Error('root resource not ready')
    yield { stage: 'fetching' }
    const { cdn } = await root.getCdn(request.cdnId, signal)
    using cdnHandle = cdn
    let copyCompleted = false
    for await (const progress of cdnHandle.copyV86ImageToSpace(
      request.sessionIndex,
      request.spaceId,
      request.cdnSourceObjectKey,
      request.imageObjectKey,
      signal,
    )) {
      if (
        progress.stage === undefined ||
        progress.stage ===
          CopyV86ImageToSpaceStage.CopyV86ImageToSpaceStage_FETCHING
      ) {
        yield { stage: 'fetching' }
        continue
      }
      copyCompleted =
        progress.stage ===
          CopyV86ImageToSpaceStage.CopyV86ImageToSpaceStage_DONE ||
        copyCompleted
      yield {
        stage: 'copying',
        blocksSeen: progress.blocksSeen,
        blocksCopied: progress.blocksCopied,
        blocksWritten: progress.blocksWritten,
        logicalSourceBytes: progress.logicalSourceBytes,
      }
    }
    if (!copyCompleted) {
      throw new Error('V86 CDN image copy ended before completion')
    }
  }

  yield { stage: 'creating' }
  const op: CreateVmV86Op = {
    objectKey: request.vmKey,
    name: request.vmName,
    timestamp: new Date(),
    imageObjectKey: request.imageObjectKey,
    config: {
      memoryMb: request.memoryMb,
      vgaMemoryMb: request.vgaMemoryMb,
      networking: request.networking,
      serialEnabled: true,
      bootArgs: '',
      mounts: [],
    },
  }
  await world.applyWorldOp(
    CREATE_VM_V86_OP_ID,
    CreateVmV86Op.toBinary(op),
    request.sessionPeerId,
    signal,
  )
  await world.deleteObject(request.wizardObjectKey, signal)
  yield { stage: 'ready' }
}

// VmV86WizardViewer is the custom wizard viewer for creating V86 VMs.
// Step 0: image source selection (existing in-space V86Image, inherit from
// existing VmV86, or copy default from CDN). Step 1: VM name and memory
// configuration. Finalize runs the CDN copy (when selected) and then
// CreateVmV86Op with the resolved image_object_key.
export function VmV86WizardViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const { spaceState, spaceWorldResource, spaceId } =
    SpaceContainerContext.useContext()
  const sessionIndex = use(SessionIndexContext)

  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)

  const ws = useWizardState({ objectInfo, worldState }, undefined)
  const {
    configData,
    handleConfigDataChange,
    handleBack,
    handleCancel,
    handleUpdateName,
    localName,
    objectKey,
    persistDraftState,
    sessionPeerId,
    state,
    wizardResource,
    navigateToObjects,
  } = ws

  const [creating, setCreating] = useState(false)
  const [cdnPickerOpen, setCdnPickerOpen] = useState(false)
  const [operationError, setOperationError] = useState('')
  const [creationRequest, setCreationRequest] =
    useState<VmCreationRequest | null>(null)
  const existingObjectKeys = useMemo(
    () =>
      spaceState.worldContents?.objects?.map((obj) => obj.objectKey ?? '') ??
      [],
    [spaceState.worldContents?.objects],
  )

  const cfg = useMemo(() => decodeConfig(configData), [configData])
  const wizardResourceStageRef = useRef(false)
  useEffect(() => {
    if (wizardResourceStageRef.current || !wizardResource.value) return
    wizardResourceStageRef.current = true
    markQuickstartStartupBoundary('v86.wizard.resource-acquired', {
      objectKey,
    })
  }, [objectKey, wizardResource.value])

  const inSpaceImagesResource = useResource(
    spaceWorldResource,
    (world: EngineWorldState, signal: AbortSignal) =>
      loadInSpaceV86Images(world, signal),
    [],
  )
  const inSpaceImages = useMemo(
    () => inSpaceImagesResource.value ?? [],
    [inSpaceImagesResource.value],
  )

  const existingVmsResource = useResource(
    spaceWorldResource,
    (world: EngineWorldState, signal: AbortSignal) =>
      loadExistingVms(world, signal),
    [],
  )
  const existingVms = useMemo(
    () => existingVmsResource.value ?? [],
    [existingVmsResource.value],
  )

  const creationResource = useStreamingResource(
    spaceWorldResource,
    async function* (world: EngineWorldState, signal: AbortSignal) {
      if (!creationRequest) return
      yield* runVmCreation(world, creationRequest, signal)
    },
    [creationRequest],
  )
  const completedCreationRef = useRef<VmCreationRequest | null>(null)
  useEffect(() => {
    if (creationResource.value?.stage !== 'ready' || !creationRequest) return
    if (completedCreationRef.current === creationRequest) return
    completedCreationRef.current = creationRequest
    toast.success(`Created ${creationRequest.vmName}`)
    navigateToObjects([creationRequest.vmKey])
  }, [creationRequest, creationResource.value, navigateToObjects])

  const existingDefault = useMemo(() => {
    for (const vm of existingVms) {
      if (vm.imageKey) return vm
    }
    return undefined
  }, [existingVms])

  const updateConfigDraft = useCallback(
    (next: V86WizardConfig) => {
      handleConfigDataChange(V86WizardConfig.toBinary(next))
    },
    [handleConfigDataChange],
  )

  const persistConfig = useCallback(
    async (next: V86WizardConfig) => {
      const handle = wizardResource.value
      if (!handle) return
      const data = V86WizardConfig.toBinary(next)
      handleConfigDataChange(data)
      await handle.updateState({ configData: data })
    },
    [handleConfigDataChange, wizardResource],
  )

  // Compute an intelligent default once the world listings are loaded and the
  // wizard has no source yet. Prefers inheriting from the newest existing VM,
  // falls back to the newest in-space V86Image, falls back to COPY_FROM_CDN
  // (the quickstart pre-seed also sets COPY_FROM_CDN explicitly).
  const seededRef = useRef(false)
  useEffect(() => {
    if (seededRef.current) return
    if (!state) return
    if (inSpaceImagesResource.loading || existingVmsResource.loading) return
    if (cfg.source !== V86WizardConfig_Source.SOURCE_UNSPECIFIED) {
      seededRef.current = true
      return
    }
    seededRef.current = true
    const next = seedV86WizardConfig(cfg, existingDefault, inSpaceImages)
    void persistConfig(next)
  }, [
    state,
    cfg,
    existingDefault,
    inSpaceImages,
    inSpaceImagesResource.loading,
    existingVmsResource.loading,
    persistConfig,
  ])

  const defaultCdnImageResource = useResource(
    rootResource,
    (nextRoot: Root, signal: AbortSignal) => {
      if (cfg.source !== V86WizardConfig_Source.COPY_FROM_CDN) {
        return Promise.resolve(undefined)
      }
      if (cfg.cdnSourceObjectKey) {
        return loadCdnV86Image(
          nextRoot,
          cfg.cdnId ?? '',
          cfg.cdnSourceObjectKey,
          signal,
        )
      }
      return discoverDefaultCdnV86Image(nextRoot, cfg.cdnId ?? '', signal)
    },
    [cfg.source, cfg.cdnId, cfg.cdnSourceObjectKey],
  )
  const catalogStageRef = useRef(false)
  useEffect(() => {
    if (
      catalogStageRef.current ||
      cfg.source !== V86WizardConfig_Source.COPY_FROM_CDN
    ) {
      return
    }
    if (defaultCdnImageResource.error) {
      catalogStageRef.current = true
      markQuickstartStartupBoundary('v86.wizard.catalog-error', {
        error: defaultCdnImageResource.error.message,
      })
      return
    }
    if (!defaultCdnImageResource.loading && defaultCdnImageResource.value) {
      catalogStageRef.current = true
      markQuickstartStartupBoundary('v86.wizard.catalog-loaded', {
        objectKey: defaultCdnImageResource.value.objectKey,
      })
    }
  }, [
    cfg.source,
    defaultCdnImageResource.error,
    defaultCdnImageResource.loading,
    defaultCdnImageResource.value,
  ])

  useEffect(() => {
    if (cfg.source !== V86WizardConfig_Source.COPY_FROM_CDN) return
    if (cfg.cdnSourceObjectKey) return
    const entry = defaultCdnImageResource.value
    if (!entry) return
    void persistConfig({
      ...cfg,
      imageObjectKey: cfg.imageObjectKey || V86_USER_IMAGE_OBJECT_KEY,
      cdnSourceObjectKey: entry.objectKey,
      cdnId: cfg.cdnId ?? '',
    })
  }, [cfg, defaultCdnImageResource.value, persistConfig])
  const nameStageRef = useRef(false)
  useEffect(() => {
    if (nameStageRef.current || state?.step !== 1) return
    nameStageRef.current = true
    markQuickstartStartupBoundary('v86.wizard.name-rendered', {
      objectKey,
    })
  }, [objectKey, state?.step])

  const selectedImage = useMemo((): V86Image | undefined => {
    if (!cfg.imageObjectKey) return undefined
    if (cfg.source === V86WizardConfig_Source.COPY_FROM_CDN) {
      return undefined
    }
    return inSpaceImages.find((e) => e.objectKey === cfg.imageObjectKey)?.image
  }, [cfg.imageObjectKey, cfg.source, inSpaceImages])

  const selectedCdnImage = useMemo((): V86Image | undefined => {
    if (cfg.source !== V86WizardConfig_Source.COPY_FROM_CDN) {
      return undefined
    }
    const entry = defaultCdnImageResource.value
    if (!entry) return undefined
    if (cfg.cdnSourceObjectKey && entry.objectKey !== cfg.cdnSourceObjectKey) {
      return undefined
    }
    return entry.image
  }, [cfg.cdnSourceObjectKey, cfg.source, defaultCdnImageResource.value])

  const handleSelectInSpaceImage = useCallback(
    (imageKey: string) => {
      setOperationError('')
      const next: V86WizardConfig = { ...cfg }
      next.source = V86WizardConfig_Source.EXISTING_IN_SPACE
      next.imageObjectKey = imageKey
      next.cdnSourceObjectKey = ''
      void (async () => {
        await persistConfig(next)
        const handle = wizardResource.value
        if (handle) await handle.updateState({ step: 1 })
      })()
    },
    [cfg, persistConfig, wizardResource],
  )

  const handlePickCdnEntry = useCallback(
    (cdnSrcKey: string) => {
      setOperationError('')
      const next: V86WizardConfig = { ...cfg }
      next.source = V86WizardConfig_Source.COPY_FROM_CDN
      next.imageObjectKey = V86_USER_IMAGE_OBJECT_KEY
      next.cdnSourceObjectKey = cdnSrcKey
      next.cdnId = next.cdnId ?? ''
      setCdnPickerOpen(false)
      void (async () => {
        await persistConfig(next)
        const handle = wizardResource.value
        if (handle) await handle.updateState({ step: 1 })
      })()
    },
    [cfg, persistConfig, wizardResource],
  )

  const handleOpenCdnPicker = useCallback(() => {
    setCdnPickerOpen(true)
  }, [])

  const handleCloseCdnPicker = useCallback(() => {
    setCdnPickerOpen(false)
  }, [])

  const handleMemoryChange = useCallback(
    (memoryMb: number) => {
      const next: V86WizardConfig = { ...cfg }
      next.memoryMb = memoryMb
      updateConfigDraft(next)
    },
    [cfg, updateConfigDraft],
  )

  const handleCancelClick = useCallback(() => {
    void handleCancel()
  }, [handleCancel])

  const handleFinalize = useCallback(async () => {
    if (!state || creating || !localName.trim()) return
    if (!cfg.imageObjectKey) {
      const message = 'Choose a VM image before creating the VM.'
      setOperationError(message)
      toast.error(message)
      return
    }
    if (
      cfg.source === V86WizardConfig_Source.COPY_FROM_CDN &&
      !cfg.cdnSourceObjectKey
    ) {
      const message = 'Choose an image from the catalog before creating the VM.'
      setOperationError(message)
      toast.error(message)
      return
    }
    if (!sessionPeerId) {
      const message = 'VM creation is not ready in this session. Try again.'
      setOperationError(message)
      toast.error(message)
      return
    }
    setOperationError('')
    setCreating(true)
    try {
      await persistDraftState()
      const copyFromCdn = cfg.source === V86WizardConfig_Source.COPY_FROM_CDN
      if (copyFromCdn) {
        if (!root) throw new Error('root resource not ready')
        if (!spaceId) throw new Error('space id not available')
      }
      setCreationRequest({
        copyFromCdn,
        cdnId: cfg.cdnId ?? '',
        cdnSourceObjectKey: cfg.cdnSourceObjectKey ?? '',
        imageObjectKey: cfg.imageObjectKey,
        sessionIndex,
        spaceId,
        vmKey: buildObjectKey('vm/v86/', localName, existingObjectKeys),
        vmName: localName,
        memoryMb: cfg.memoryMb || DEFAULT_V86_MEMORY_MB,
        vgaMemoryMb: cfg.vgaMemoryMb || DEFAULT_V86_VGA_MEMORY_MB,
        networking: cfg.networking ?? false,
        sessionPeerId,
        wizardObjectKey: objectKey,
        root: root ?? undefined,
      })
    } catch {
      const message = 'VM creation could not start. Try again.'
      setOperationError(message)
      setCreating(false)
      toast.error(message)
    }
  }, [
    state,
    creating,
    localName,
    cfg,
    sessionPeerId,
    root,
    spaceId,
    sessionIndex,
    objectKey,
    existingObjectKeys,
    persistDraftState,
  ])

  const handleFinalizeClick = useCallback(() => {
    void handleFinalize()
  }, [handleFinalize])

  if (creationRequest) {
    const progress =
      creationResource.value ??
      ({
        stage: creationRequest.copyFromCdn ? 'fetching' : 'creating',
      } satisfies VmCreationProgress)
    const error = !creationResource.error
      ? undefined
      : progress.stage === 'fetching'
        ? 'The image could not be fetched. Check your connection and try again.'
        : progress.stage === 'copying'
          ? 'The image copy stopped. Try again to continue.'
          : 'The image is ready, but the VM could not be created. Try again.'
    return (
      <VmCreationProgressScreen
        progress={progress}
        vmName={creationRequest.vmName}
        includesCdnCopy={creationRequest.copyFromCdn}
        error={error}
        onRetry={error ? creationResource.retry : undefined}
      />
    )
  }

  if (!state) {
    return (
      <div className="flex flex-1 items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading wizard',
              detail: 'Preparing the VM creation workflow.',
            }}
          />
        </div>
      </div>
    )
  }

  const step = state.step ?? 0
  const memoryMb = cfg.memoryMb || DEFAULT_V86_MEMORY_MB
  const canFinalize =
    !!cfg.imageObjectKey &&
    (cfg.source !== V86WizardConfig_Source.COPY_FROM_CDN ||
      !!cfg.cdnSourceObjectKey)

  return (
    <>
      <WizardShell
        title={
          <>
            <LuMonitor className="mr-2 size-4 shrink-0" />
            New V86 VM
          </>
        }
        step={step}
        totalSteps={2}
        stepName={step === 0 ? 'Choose image' : 'Configure VM'}
        localName={localName}
        onUpdateName={handleUpdateName}
        onBack={() => void handleBack()}
        onCancel={handleCancelClick}
        nameLabel="VM Name"
        namePlaceholder="e.g. debian-lab"
        nameStep={1}
        creating={creating}
        creatingLabel="Starting VM creation…"
        onFinalize={handleFinalizeClick}
        canFinalize={canFinalize}
        finalizeStep={1}
      >
        {step === 0 && (
          <SourcePickerStep
            cfg={cfg}
            existingDefault={existingDefault}
            inSpaceImages={inSpaceImages}
            onSelectInSpace={handleSelectInSpaceImage}
            onOpenCdnPicker={handleOpenCdnPicker}
            pending={
              inSpaceImagesResource.loading || existingVmsResource.loading
            }
          />
        )}
        {step === 1 && (
          <ConfigStep
            cfg={cfg}
            memoryMb={memoryMb}
            onMemoryChange={handleMemoryChange}
            selectedImage={selectedImage}
            selectedCdnImage={selectedCdnImage}
            existingDefault={existingDefault}
          />
        )}
        {operationError && (
          <div
            className="border-destructive/15 bg-destructive/5 text-destructive rounded-lg border p-3 text-xs leading-relaxed"
            role="alert"
          >
            <div className="font-medium">VM could not be created</div>
            <div className="mt-0.5">{operationError}</div>
          </div>
        )}
      </WizardShell>
      {cdnPickerOpen && (
        <CdnImagePickerModal
          onClose={handleCloseCdnPicker}
          onSelect={handlePickCdnEntry}
          cdnId={cfg.cdnId ?? ''}
        />
      )}
    </>
  )
}

interface SourcePickerStepProps {
  cfg: V86WizardConfig
  existingDefault: ExistingVmInfo | undefined
  inSpaceImages: InSpaceV86ImageEntry[]
  onSelectInSpace: (imageKey: string) => void
  onOpenCdnPicker: () => void
  pending: boolean
}

function SourcePickerStep({
  cfg,
  existingDefault,
  inSpaceImages,
  onSelectInSpace,
  onOpenCdnPicker,
  pending,
}: SourcePickerStepProps) {
  const shortcutRow = existingDefault?.imageKey ? (
    <button
      type="button"
      className={cn(
        'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 flex w-full items-center gap-3 rounded-lg border p-3 text-left transition-all duration-150',
        cfg.source === V86WizardConfig_Source.EXISTING_IN_SPACE &&
          cfg.imageObjectKey === existingDefault.imageKey &&
          'border-brand/30 bg-brand/10',
      )}
      onClick={() => onSelectInSpace(existingDefault.imageKey)}
    >
      <span className="bg-foreground/5 flex size-7 shrink-0 items-center justify-center rounded-md">
        <LuRefreshCcw className="text-foreground-alt/50 size-3.5" />
      </span>
      <div className="min-w-0">
        <div className="text-foreground text-sm font-medium">
          Use same image as {existingDefault.name}
        </div>
        <div className="text-foreground-alt/50 text-xs">
          Inherit the V86Image from the newest existing VM in this Space.
        </div>
      </div>
    </button>
  ) : null

  return (
    <section>
      <div className="mb-2 flex items-center justify-between">
        <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
          <LuMonitor className="size-3.5" />
          Choose a VM image
        </h3>
      </div>
      <div className="space-y-2">
        {pending &&
          inSpaceImages.length === 0 &&
          !existingDefault?.imageKey && (
            <LoadingCard
              view={{
                state: 'active',
                title: 'Looking for VM images in this Space…',
                detail: 'Reading images that are ready to use.',
              }}
            />
          )}
        {shortcutRow}
        {inSpaceImages.map((entry) => (
          <button
            type="button"
            key={entry.objectKey}
            className={cn(
              'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 flex w-full items-center gap-3 rounded-lg border p-3 text-left transition-all duration-150',
              cfg.source === V86WizardConfig_Source.EXISTING_IN_SPACE &&
                cfg.imageObjectKey === entry.objectKey &&
                'border-brand/30 bg-brand/10',
            )}
            onClick={() => onSelectInSpace(entry.objectKey)}
          >
            <span className="bg-foreground/5 flex size-7 shrink-0 items-center justify-center rounded-md">
              <LuHardDrive className="text-foreground-alt/50 size-3.5" />
            </span>
            <div className="min-w-0">
              <div className="text-foreground text-sm font-medium">
                {formatImageLabel(entry.image)}
              </div>
              <div className="text-foreground-alt/50 truncate text-xs">
                {entry.image.distro || entry.objectKey}
              </div>
            </div>
          </button>
        ))}
        {!pending &&
          (inSpaceImages.length > 0 || existingDefault?.imageKey) && (
            <button
              type="button"
              className={cn(
                'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 flex w-full items-center gap-3 rounded-lg border p-3 text-left transition-all duration-150',
                cfg.source === V86WizardConfig_Source.COPY_FROM_CDN &&
                  'border-brand/30 bg-brand/5',
              )}
              onClick={onOpenCdnPicker}
            >
              <span className="bg-brand/10 flex size-7 shrink-0 items-center justify-center rounded-md">
                <LuCloud className="text-brand size-3.5" />
              </span>
              <div className="min-w-0">
                <div className="text-foreground text-sm font-medium">
                  Add image from catalog
                </div>
                <div className="text-foreground-alt/70 text-xs leading-relaxed">
                  Copy a published VM image into this Space.
                </div>
              </div>
            </button>
          )}
      </div>
      {!pending && inSpaceImages.length === 0 && !existingDefault?.imageKey && (
        <InfoCard
          icon={<LuHardDrive className="text-foreground-alt/60 size-3.5" />}
          title="No VM images in this Space"
        >
          <p className="text-foreground-alt/70 text-xs leading-relaxed">
            Copy a published image from the catalog to continue.
          </p>
          <Button
            size="sm"
            onClick={onOpenCdnPicker}
            className="border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 text-foreground mt-3 h-7 rounded-md border px-3 text-xs"
          >
            Browse image catalog
          </Button>
        </InfoCard>
      )}
    </section>
  )
}

interface ConfigStepProps {
  cfg: V86WizardConfig
  memoryMb: number
  onMemoryChange: (memoryMb: number) => void
  selectedImage: V86Image | undefined
  selectedCdnImage: V86Image | undefined
  existingDefault: ExistingVmInfo | undefined
}

function ConfigStep({
  cfg,
  memoryMb,
  onMemoryChange,
  selectedImage,
  selectedCdnImage,
  existingDefault,
}: ConfigStepProps) {
  const isCdn = cfg.source === V86WizardConfig_Source.COPY_FROM_CDN
  const imageSummary = isCdn
    ? selectedCdnImage
      ? `Will copy from catalog: ${formatImageLabel(selectedCdnImage)}`
      : `Catalog image: ${cfg.cdnSourceObjectKey || 'Not selected'}`
    : selectedImage
      ? formatImageLabel(selectedImage)
      : existingDefault?.imageKey
        ? `Using ${existingDefault.imageKey} from ${existingDefault.name}`
        : cfg.imageObjectKey || 'Not selected'

  return (
    <div className="space-y-3">
      <div className="border-foreground/6 bg-background-card/30 space-y-3 rounded-lg border p-3.5">
        <section>
          <div className="mb-2 flex items-center justify-between">
            <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
              <LuCpu className="size-3.5" />
              Memory
            </h3>
          </div>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
            {MEMORY_OPTIONS.map((mb) => (
              <button
                type="button"
                key={mb}
                className={cn(
                  'border-foreground/10 bg-background/20 text-foreground-alt hover:border-foreground/20 hover:bg-background/30 rounded-md border px-3 py-2 text-left text-xs transition-all duration-150 select-none',
                  memoryMb === mb &&
                    'border-brand/30 bg-brand/10 text-foreground',
                )}
                onClick={() => onMemoryChange(mb)}
              >
                {mb} MB
              </button>
            ))}
          </div>
        </section>
        <section className="border-foreground/8 border-t pt-3">
          <div className="text-foreground mb-2 flex items-center gap-1.5 text-xs font-medium">
            <LuHardDrive className="size-3.5" />
            Image
          </div>
          <div className="text-foreground text-sm font-medium">
            {imageSummary}
          </div>
          <p className="text-foreground-alt/70 mt-1 text-xs leading-relaxed">
            {isCdn
              ? 'The image is copied into this Space before the VM opens.'
              : 'The VM uses an image already stored in this Space.'}
          </p>
        </section>
      </div>
      {!cfg.imageObjectKey && (
        <div
          className="border-destructive/15 bg-destructive/5 text-destructive rounded-lg border p-3 text-xs leading-relaxed"
          role="alert"
        >
          Choose a VM image before creating the VM.
        </div>
      )}
    </div>
  )
}

interface CdnImagePickerModalProps {
  cdnId: string
  onSelect: (cdnSrcKey: string) => void
  onClose: () => void
}

function CdnImagePickerModal({
  cdnId,
  onSelect,
  onClose,
}: CdnImagePickerModalProps) {
  const rootResource = useRootResource()
  const cdnSpaceResource = useResource(
    rootResource,
    async (root: Root, signal: AbortSignal, cleanup) =>
      cleanup(await mountCdnImageSpace(root, cdnId, signal)),
    [cdnId],
  )
  const entriesResource = useStreamingResource(
    cdnSpaceResource,
    async function* (handle: CdnImageSpaceHandle, signal: AbortSignal) {
      yield await loadCdnV86ImagesFromSpace(handle.space, signal)
      for await (const _state of handle.space.watchSpaceState({}, signal)) {
        yield await loadCdnV86ImagesFromSpace(handle.space, signal)
      }
    },
    [],
  )
  const entries = entriesResource.value
  const loadError = cdnSpaceResource.error ?? entriesResource.error
  const errorCopy = loadError ? getV86CatalogErrorCopy(loadError) : undefined
  const handleRetry = () => {
    if (cdnSpaceResource.error) cdnSpaceResource.retry()
    if (entriesResource.error) entriesResource.retry()
  }

  return (
    <div
      role="presentation"
      className="bg-background/80 fixed inset-0 z-50 flex items-center justify-center p-6"
      onClick={onClose}
    >
      <div
        role="presentation"
        className="border-foreground/8 bg-background-card/95 flex max-h-[80vh] w-full max-w-md flex-col gap-3 rounded-xl border p-4 shadow-lg backdrop-blur-sm"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex items-center justify-between gap-3">
          <h3 className="text-foreground text-sm font-semibold tracking-tight select-none">
            VM image catalog
          </h3>
          <Button
            variant="outline"
            size="sm"
            onClick={onClose}
            className="border-foreground/8 hover:border-foreground/15 hover:bg-foreground/5 text-foreground-alt hover:text-foreground h-7 bg-transparent px-2 text-xs transition-all duration-150"
          >
            Close
          </Button>
        </div>
        <div className="space-y-2 overflow-y-auto">
          {!entries && !loadError && (
            <LoadingCard
              view={{
                state: 'active',
                title: 'Loading image catalog',
                detail: 'Looking for published VM images.',
              }}
            />
          )}
          {errorCopy && (
            <div
              className="border-destructive/15 bg-destructive/5 rounded-lg border p-3"
              role="alert"
            >
              <div className="text-destructive text-sm font-semibold">
                {errorCopy.title}
              </div>
              <p className="text-destructive mt-1 text-xs leading-relaxed">
                {errorCopy.detail}
              </p>
              {!errorCopy.unpublished && (
                <Button
                  size="sm"
                  onClick={handleRetry}
                  className="border-destructive/20 bg-destructive/10 text-destructive hover:bg-destructive/15 mt-3 h-7 rounded-md border px-3 text-xs"
                >
                  Retry
                </Button>
              )}
            </div>
          )}
          {entries && entries.length === 0 && !loadError && (
            <InfoCard
              icon={<LuCloud className="text-foreground-alt/60 size-3.5" />}
              title="No VM images are published yet"
            >
              <p className="text-foreground-alt/70 text-xs leading-relaxed">
                This image catalog has no published images to copy.
              </p>
            </InfoCard>
          )}
          {entries?.map((entry) => (
            <button
              type="button"
              key={entry.objectKey}
              className="border-foreground/10 bg-background-card/30 hover:border-foreground/20 hover:bg-background-card/50 flex w-full flex-col items-start gap-1 rounded-lg border p-3 text-left transition-all duration-150"
              onClick={() => onSelect(entry.objectKey)}
            >
              <span className="text-foreground text-sm font-medium">
                {formatImageLabel(entry.image)}
              </span>
              <span className="text-foreground-alt/60 text-xs leading-relaxed">
                {entry.metadataError
                  ? 'Image details are unavailable.'
                  : entry.image.distro || ''}
                {!entry.metadataError && entry.image.tags?.length
                  ? ` · ${entry.image.tags.join(', ')}`
                  : ''}
              </span>
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}
