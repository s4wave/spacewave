import {
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
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
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import { listObjectsWithType } from '@s4wave/sdk/world/types/types.js'
import { keyToIRI, iriToKey } from '@s4wave/sdk/world/graph-utils.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import type { Cdn } from '@s4wave/sdk/cdn/cdn.js'
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
import {
  compareV86ImageNewestFirst,
  DEFAULT_V86_MEMORY_MB,
  DEFAULT_V86_VGA_MEMORY_MB,
  isDefaultV86Image,
  seedV86WizardConfig,
  V86_USER_IMAGE_OBJECT_KEY,
} from '../vm/v86-wizard-config.js'

import { WizardShell } from './WizardShell.js'
import { useWizardState } from './useWizardState.js'

// VmV86WizardTypeID is the wizard block type id for v86 VM creation wizards.
export const VmV86WizardTypeID = 'wizard/v86'

// VmV86TypeID mirrors sdk/vm/v86.go VmV86TypeID; keep these aligned.
const VmV86TypeID = 'spacewave/vm/v86'

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

async function loadCdnV86ImagesFromSpace(
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
    const resp = await cursor.unmarshal({}, signal)
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

async function loadCdnV86Images(
  root: Root,
  cdnId: string,
  signal: AbortSignal,
): Promise<CdnV86ImageEntry[]> {
  using handle = await mountCdnImageSpace(root, cdnId, signal)
  return loadCdnV86ImagesFromSpace(handle.space, signal)
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
    const resp = await cursor.unmarshal({}, signal)
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
    const resp = await cursor.unmarshal({}, signal)
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
  const sessionIndex = useContext(SessionIndexContext)

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
    spaceWorld,
    state,
    wizardResource,
    navigateToObjects,
  } = ws

  const [creating, setCreating] = useState(false)
  const [cdnPickerOpen, setCdnPickerOpen] = useState(false)
  const existingObjectKeys = useMemo(
    () =>
      spaceState.worldContents?.objects?.map((obj) => obj.objectKey ?? '') ??
      [],
    [spaceState.worldContents?.objects],
  )

  const cfg = useMemo(() => decodeConfig(configData), [configData])

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
        return Promise.resolve(undefined)
      }
      return discoverDefaultCdnV86Image(nextRoot, cfg.cdnId ?? '', signal)
    },
    [cfg.source, cfg.cdnId, cfg.cdnSourceObjectKey],
  )

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
      toast.error('Select a VM image source before creating.')
      return
    }
    if (
      cfg.source === V86WizardConfig_Source.COPY_FROM_CDN &&
      !cfg.cdnSourceObjectKey
    ) {
      toast.error('Pick a CDN image to copy before creating.')
      return
    }
    if (!sessionPeerId) {
      toast.error('Session peer id not available; cannot create VM.')
      return
    }
    setCreating(true)
    try {
      await persistDraftState()
      if (cfg.source === V86WizardConfig_Source.COPY_FROM_CDN) {
        if (!root) throw new Error('root resource not ready')
        if (!spaceId) throw new Error('space id not available')
        const { cdn } = await root.getCdn(cfg.cdnId ?? '')
        using cdnHandle = cdn
        await cdnHandle.copyV86ImageToSpace(
          sessionIndex,
          spaceId,
          cfg.cdnSourceObjectKey ?? '',
          cfg.imageObjectKey,
        )
      }
      const vmKey = buildObjectKey('vm/v86/', localName, existingObjectKeys)
      const op: CreateVmV86Op = {
        objectKey: vmKey,
        name: localName,
        timestamp: new Date(),
        imageObjectKey: cfg.imageObjectKey,
        config: {
          memoryMb: cfg.memoryMb || DEFAULT_V86_MEMORY_MB,
          vgaMemoryMb: cfg.vgaMemoryMb || DEFAULT_V86_VGA_MEMORY_MB,
          networking: cfg.networking ?? false,
          serialEnabled: true,
          bootArgs: '',
          mounts: [],
        },
      }
      const opData = CreateVmV86Op.toBinary(op)
      await spaceWorld.applyWorldOp(CREATE_VM_V86_OP_ID, opData, sessionPeerId)
      await spaceWorld.deleteObject(objectKey)
      toast.success(`Created ${localName}`)
      navigateToObjects([vmKey])
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to create V86 VM',
      )
    } finally {
      setCreating(false)
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
    spaceWorld,
    objectKey,
    navigateToObjects,
    existingObjectKeys,
    persistDraftState,
  ])

  const handleFinalizeClick = useCallback(() => {
    void handleFinalize()
  }, [handleFinalize])

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
            <LuMonitor className="mr-2 h-4 w-4 shrink-0" />
            New V86 VM
          </>
        }
        step={step}
        totalSteps={2}
        localName={localName}
        onUpdateName={handleUpdateName}
        onBack={() => void handleBack()}
        onCancel={handleCancelClick}
        nameLabel="VM Name"
        namePlaceholder="e.g. debian-lab"
        nameStep={1}
        creating={creating}
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
  const shortcutRow =
    existingDefault?.imageKey ?
      <button
        type="button"
        className={cn(
          'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 flex w-full items-center gap-3 rounded-lg border p-3 text-left transition-all duration-150',
          cfg.source === V86WizardConfig_Source.EXISTING_IN_SPACE &&
            cfg.imageObjectKey === existingDefault.imageKey &&
            'border-brand/30 bg-brand/5',
        )}
        onClick={() => onSelectInSpace(existingDefault.imageKey)}
      >
        <span className="bg-foreground/5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md">
          <LuRefreshCcw className="text-foreground-alt/50 h-3.5 w-3.5" />
        </span>
        <div className="min-w-0">
          <div className="text-foreground text-xs font-medium">
            Use same image as {existingDefault.name}
          </div>
          <div className="text-foreground-alt/50 text-xs">
            Inherit the V86Image from the newest existing VM in this Space.
          </div>
        </div>
      </button>
    : null

  return (
    <section>
      <div className="mb-2 flex items-center justify-between">
        <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
          <LuMonitor className="h-3.5 w-3.5" />
          Choose a VM image
        </h3>
      </div>
      <div className="space-y-2">
        {shortcutRow}
        {inSpaceImages.map((entry) => (
          <button
            type="button"
            key={entry.objectKey}
            className={cn(
              'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 flex w-full items-center gap-3 rounded-lg border p-3 text-left transition-all duration-150',
              cfg.source === V86WizardConfig_Source.EXISTING_IN_SPACE &&
                cfg.imageObjectKey === entry.objectKey &&
                'border-brand/30 bg-brand/5',
            )}
            onClick={() => onSelectInSpace(entry.objectKey)}
          >
            <span className="bg-foreground/5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md">
              <LuHardDrive className="text-foreground-alt/50 h-3.5 w-3.5" />
            </span>
            <div className="min-w-0">
              <div className="text-foreground text-xs font-medium">
                {formatImageLabel(entry.image)}
              </div>
              <div className="text-foreground-alt/50 truncate text-xs">
                {entry.image.distro || entry.objectKey}
              </div>
            </div>
          </button>
        ))}
        <button
          type="button"
          className={cn(
            'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 flex w-full items-center gap-3 rounded-lg border p-3 text-left transition-all duration-150',
            cfg.source === V86WizardConfig_Source.COPY_FROM_CDN &&
              'border-brand/30 bg-brand/5',
          )}
          onClick={onOpenCdnPicker}
        >
          <span className="bg-brand/10 flex h-7 w-7 shrink-0 items-center justify-center rounded-md">
            <LuCloud className="text-brand h-3.5 w-3.5" />
          </span>
          <div className="min-w-0">
            <div className="text-foreground text-xs font-medium">
              Copy default from CDN
            </div>
            <div className="text-foreground-alt/50 text-xs">
              Download a published Aperture V86Image into this Space.
            </div>
          </div>
        </button>
      </div>
      {pending && (
        <div className="mt-2">
          <LoadingInline
            label="Loading images from this Space"
            tone="muted"
            size="sm"
          />
        </div>
      )}
      {!pending && inSpaceImages.length === 0 && !existingDefault?.imageKey && (
        <div className="border-foreground/6 bg-background-card/30 text-foreground-alt/40 mt-2 flex items-center gap-2 rounded-lg border px-3.5 py-3 text-xs">
          <LuHardDrive className="h-3.5 w-3.5 shrink-0" />
          No V86Images exist in this Space yet. Copy one from the CDN to
          continue.
        </div>
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
  return (
    <div className="space-y-3">
      <section>
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
            <LuCpu className="h-3.5 w-3.5" />
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
                memoryMb === mb && 'border-brand/30 bg-brand/5 text-foreground',
              )}
              onClick={() => onMemoryChange(mb)}
            >
              {mb} MB
            </button>
          ))}
        </div>
      </section>
      <div className="border-foreground/6 bg-background-card/30 flex items-start gap-3 rounded-lg border p-3.5">
        <span className="bg-foreground/5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md">
          <LuCpu className="text-foreground-alt/50 h-3.5 w-3.5" />
        </span>
        <div className="flex flex-col gap-0.5">
          <div className="text-foreground text-xs font-medium select-none">
            Image
          </div>
          <div className="text-foreground-alt/50 text-xs">
            {isCdn ?
              selectedCdnImage ?
                `Will copy from CDN: ${formatImageLabel(selectedCdnImage)}`
              : `Will copy from CDN: ${cfg.cdnSourceObjectKey || '(pending)'}`
            : selectedImage ?
              formatImageLabel(selectedImage)
            : existingDefault?.imageKey ?
              `Inheriting image ${existingDefault.imageKey} from ${existingDefault.name}`
            : cfg.imageObjectKey || '(no image selected)'}
          </div>
        </div>
      </div>
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

  return (
    <div
      className="bg-background/80 fixed inset-0 z-50 flex items-center justify-center p-6"
      onClick={onClose}
    >
      <div
        className="border-foreground/8 bg-background-card/95 flex max-h-[80vh] w-full max-w-md flex-col gap-3 rounded-xl border p-4 shadow-lg backdrop-blur-sm"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between">
          <h3 className="text-foreground text-sm font-semibold tracking-tight select-none">
            Pick a CDN V86Image
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
            <LoadingInline label="Loading" tone="muted" size="sm" />
          )}
          {loadError && (
            <span className="text-destructive text-xs">
              {loadError.message}
            </span>
          )}
          {entries && entries.length === 0 && !loadError && (
            <span className="text-foreground-alt text-xs">
              No V86Images published in this CDN yet.
            </span>
          )}
          {entries?.map((entry) => (
            <button
              type="button"
              key={entry.objectKey}
              className="border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 flex w-full flex-col items-start gap-1 rounded-lg border p-3 text-left transition-all duration-150"
              onClick={() => onSelect(entry.objectKey)}
            >
              <span className="text-foreground text-xs font-medium">
                {formatImageLabel(entry.image)}
              </span>
              <span className="text-foreground-alt/50 text-xs">
                {entry.metadataError ?
                  `Metadata decode failed: ${entry.metadataError}`
                : entry.image.distro || ''}
                {!entry.metadataError && entry.image.tags?.length ?
                  `  ·  ${entry.image.tags.join(', ')}`
                : ''}
              </span>
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}
