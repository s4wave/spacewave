import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { LuMonitor } from '@s4wave/web/images/icons.js'

import {
  useResource,
  useResourceValue,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { Root } from '@s4wave/sdk/root/root.js'
import type { V86Image } from '@s4wave/sdk/vm/v86.pb.js'
import {
  V86WizardConfig,
  V86WizardConfig_Source,
} from '@s4wave/sdk/vm/v86-wizard.pb.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { markQuickstartStartupBoundary } from '../quickstart/startup-boundary.js'
import {
  buildVmCreationRequest,
  finalizeBlocker,
  listExistingVms,
  runVmCreation,
  type ExistingVm,
  type VmCreationProgress,
  type VmCreationRequest,
  type VmCreationStage,
} from '../vm/v86-creation.js'
import {
  listV86Images,
  loadCdnV86Image,
  type V86ImageEntry,
} from '../vm/v86-images.js'
import {
  DEFAULT_V86_MEMORY_MB,
  seedV86WizardConfig,
  V86_USER_IMAGE_OBJECT_KEY,
} from '../vm/v86-wizard-config.js'
import { V86CatalogDialog } from './V86CatalogDialog.js'
import { V86ConfigStep } from './V86ConfigStep.js'
import { V86SourceStep } from './V86SourceStep.js'
import { VmCreationProgressScreen } from './VmCreationProgressScreen.js'
import { WizardShell } from './WizardShell.js'
import { useWizardState } from './useWizardState.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule2: { width: '100%', maxWidth: 'var(--container-sm)' },
  rule3: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-destructive'],
  },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule5: { marginTop: `calc(${themeLayout['--spacing']} * .5)` },
})

const stylexStyles = stylex.create({
  rule1: {
    marginRight: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
  },
})

// decodeConfig reads the wizard config, or an empty one when it is missing or
// does not decode.
function decodeConfig(configData: Uint8Array | undefined): V86WizardConfig {
  if (!configData?.length) return V86WizardConfig.create({})
  try {
    return V86WizardConfig.fromBinary(configData)
  } catch {
    return V86WizardConfig.create({})
  }
}

type WizardState = ReturnType<typeof useWizardState>
type WizardResource = WizardState['wizardResource']
type SpaceWorldResource = ReturnType<
  typeof SpaceContainerContext.useContext
>['spaceWorldResource']
type VmCreationResource = ReturnType<typeof useVmCreation>['creationResource']

/**
 * useV86WizardStartupMarks records the quickstart startup boundaries the
 * wizard reaches: the wizard resource acquired and the name step rendered.
 */
function useV86WizardStartupMarks(
  objectKey: string,
  wizardAcquired: boolean,
  step: number | undefined,
) {
  const acquiredMarkedRef = useRef(false)
  useEffect(() => {
    if (acquiredMarkedRef.current || !wizardAcquired) return
    acquiredMarkedRef.current = true
    markQuickstartStartupBoundary('v86.wizard.resource-acquired', {
      objectKey,
    })
  }, [objectKey, wizardAcquired])

  const nameMarkedRef = useRef(false)
  useEffect(() => {
    if (nameMarkedRef.current || step !== 1) return
    nameMarkedRef.current = true
    markQuickstartStartupBoundary('v86.wizard.name-rendered', {
      objectKey,
    })
  }, [objectKey, step])
}

/**
 * useV86WorldListings loads the VM images and VMs already in the Space and
 * resolves the newest VM that has an image, which new VMs inherit from.
 */
function useV86WorldListings(spaceWorldResource: SpaceWorldResource) {
  const inSpaceImagesResource = useResource(
    spaceWorldResource,
    (world: EngineWorldState, signal: AbortSignal) =>
      listV86Images(world, signal),
    [],
  )
  const inSpaceImages = useMemo(
    () => inSpaceImagesResource.value ?? [],
    [inSpaceImagesResource.value],
  )

  const existingVmsResource = useResource(
    spaceWorldResource,
    (world: EngineWorldState, signal: AbortSignal) =>
      listExistingVms(world, signal),
    [],
  )
  const existingDefault = useMemo(
    () => existingVmsResource.value?.find((vm) => vm.imageKey),
    [existingVmsResource.value],
  )

  return {
    inSpaceImages,
    existingDefault,
    pending: inSpaceImagesResource.loading || existingVmsResource.loading,
  }
}

/**
 * useVmCreation runs the creation stream for the request started by the
 * wizard and navigates to the new VM once it is ready.
 */
function useVmCreation() {
  const { spaceWorldResource, navigateToObjects } =
    SpaceContainerContext.useContext()
  const [creationRequest, setCreationRequest] =
    useState<VmCreationRequest | null>(null)

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

  return {
    creationRequest,
    creationResource,
    startCreation: setCreationRequest,
  }
}

/**
 * useV86ConfigWriter returns the writers for the wizard config: a draft
 * update and a persisted update through the wizard resource.
 */
function useV86ConfigWriter(
  wizardResource: WizardResource,
  handleConfigDataChange: WizardState['handleConfigDataChange'],
) {
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

  return { updateConfigDraft, persistConfig }
}

interface UseV86SeedArgs {
  ready: boolean
  cfg: V86WizardConfig
  existingDefault: ExistingVm | undefined
  inSpaceImages: V86ImageEntry[]
  listingsPending: boolean
  persistConfig: (next: V86WizardConfig) => Promise<void>
}

/**
 * useV86Seed computes an intelligent default once the world listings are
 * loaded and the wizard has no source yet. Prefers inheriting from the newest
 * existing VM, falls back to the newest image in the Space, falls back to
 * LINK_FROM_CDN (the quickstart pre-seed also sets LINK_FROM_CDN explicitly).
 */
function useV86Seed({
  ready,
  cfg,
  existingDefault,
  inSpaceImages,
  listingsPending,
  persistConfig,
}: UseV86SeedArgs) {
  const seededRef = useRef(false)
  useEffect(() => {
    if (seededRef.current || !ready || listingsPending) return
    seededRef.current = true
    if (cfg.source !== V86WizardConfig_Source.SOURCE_UNSPECIFIED) return
    void persistConfig(seedV86WizardConfig(cfg, existingDefault, inSpaceImages))
  }, [
    ready,
    cfg,
    existingDefault,
    inSpaceImages,
    listingsPending,
    persistConfig,
  ])
}

/**
 * useDefaultCdnImage resolves the CDN image for a LINK_FROM_CDN config: the
 * chosen catalog entry, or the discovered default, which is then written back
 * to the config. It records the catalog startup boundary once.
 */
function useDefaultCdnImage(
  rootResource: ReturnType<typeof useRootResource>,
  cfg: V86WizardConfig,
  persistConfig: (next: V86WizardConfig) => Promise<void>,
) {
  const defaultCdnImageResource = useResource(
    rootResource,
    (root: Root, signal: AbortSignal) => {
      if (cfg.source !== V86WizardConfig_Source.LINK_FROM_CDN) {
        return Promise.resolve(undefined)
      }
      return loadCdnV86Image(
        root,
        cfg.cdnId ?? '',
        cfg.cdnSourceObjectKey ?? '',
        signal,
      )
    },
    [cfg.source, cfg.cdnId, cfg.cdnSourceObjectKey],
  )

  const catalogMarkedRef = useRef(false)
  useEffect(() => {
    if (
      catalogMarkedRef.current ||
      cfg.source !== V86WizardConfig_Source.LINK_FROM_CDN
    ) {
      return
    }
    if (defaultCdnImageResource.error) {
      catalogMarkedRef.current = true
      markQuickstartStartupBoundary('v86.wizard.catalog-error', {
        error: defaultCdnImageResource.error.message,
      })
      return
    }
    if (!defaultCdnImageResource.loading && defaultCdnImageResource.value) {
      catalogMarkedRef.current = true
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
    if (cfg.source !== V86WizardConfig_Source.LINK_FROM_CDN) return
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

  return defaultCdnImageResource.value
}

/** selectedCdnImage returns the resolved CDN image when it is the chosen one. */
function selectedCdnImage(
  cfg: V86WizardConfig,
  entry: V86ImageEntry | null | undefined,
): V86Image | undefined {
  if (cfg.source !== V86WizardConfig_Source.LINK_FROM_CDN) return undefined
  if (!entry) return undefined
  if (cfg.cdnSourceObjectKey && entry.objectKey !== cfg.cdnSourceObjectKey) {
    return undefined
  }
  return entry.image
}

interface UseV86SourceSelectionArgs {
  cfg: V86WizardConfig
  wizardResource: WizardResource
  persistConfig: (next: V86WizardConfig) => Promise<void>
  setOperationError: (message: string) => void
}

/**
 * useV86SourceSelection handles the image source step: picking an in-space
 * image or a CDN catalog entry persists the config and advances to step 1.
 */
function useV86SourceSelection({
  cfg,
  wizardResource,
  persistConfig,
  setOperationError,
}: UseV86SourceSelectionArgs) {
  const [catalogOpen, setCatalogOpen] = useState(false)

  const persistAndAdvance = useCallback(
    (next: V86WizardConfig) => {
      void (async () => {
        await persistConfig(next)
        const handle = wizardResource.value
        if (handle) await handle.updateState({ step: 1 })
      })()
    },
    [persistConfig, wizardResource],
  )

  const selectInSpaceImage = useCallback(
    (imageKey: string) => {
      setOperationError('')
      persistAndAdvance({
        ...cfg,
        source: V86WizardConfig_Source.EXISTING_IN_SPACE,
        imageObjectKey: imageKey,
        cdnSourceObjectKey: '',
      })
    },
    [cfg, persistAndAdvance, setOperationError],
  )

  const pickCdnEntry = useCallback(
    (cdnSrcKey: string) => {
      setOperationError('')
      setCatalogOpen(false)
      persistAndAdvance({
        ...cfg,
        source: V86WizardConfig_Source.LINK_FROM_CDN,
        imageObjectKey: V86_USER_IMAGE_OBJECT_KEY,
        cdnSourceObjectKey: cdnSrcKey,
        cdnId: cfg.cdnId ?? '',
      })
    },
    [cfg, persistAndAdvance, setOperationError],
  )

  return { catalogOpen, setCatalogOpen, selectInSpaceImage, pickCdnEntry }
}

interface UseV86FinalizeArgs {
  ready: boolean
  cfg: V86WizardConfig
  localName: string
  sessionPeerId: string
  objectKey: string
  cdnImage: V86Image | undefined
  persistDraftState: () => Promise<void>
  startCreation: (request: VmCreationRequest) => void
  setOperationError: (message: string) => void
}

/**
 * useV86Finalize validates the config, persists the draft, and starts VM
 * creation. Failures surface as the operation error and a toast.
 */
function useV86Finalize({
  ready,
  cfg,
  localName,
  sessionPeerId,
  objectKey,
  cdnImage,
  persistDraftState,
  startCreation,
  setOperationError,
}: UseV86FinalizeArgs) {
  const { spaceWorld } = SpaceContainerContext.useContext()
  const root = useResourceValue(useRootResource())
  const [creating, setCreating] = useState(false)

  const fail = useCallback(
    (message: string) => {
      setOperationError(message)
      toast.error(message)
    },
    [setOperationError],
  )

  const finalize = useCallback(async () => {
    if (!ready || creating || !localName.trim()) return
    const blocker = finalizeBlocker(cfg, cdnImage, sessionPeerId)
    if (blocker) {
      fail(blocker)
      return
    }

    setOperationError('')
    setCreating(true)
    try {
      await persistDraftState()
      startCreation(
        await buildVmCreationRequest({
          cfg,
          vmName: localName,
          sessionPeerId,
          wizardObjectKey: objectKey,
          root,
          cdnImage,
          spaceWorld,
        }),
      )
    } catch (err) {
      setCreating(false)
      const cause = err instanceof Error ? err.message : String(err)
      fail('VM creation could not start: ' + cause)
    }
  }, [
    ready,
    creating,
    localName,
    cfg,
    sessionPeerId,
    root,
    cdnImage,
    objectKey,
    spaceWorld,
    persistDraftState,
    startCreation,
    fail,
    setOperationError,
  ])

  const finalizeClick = useCallback(() => {
    void finalize()
  }, [finalize])

  return { creating, finalizeClick }
}

/** creationFailureMessage describes the failed creation stage for the user. */
function creationFailureMessage(stage: VmCreationStage): string {
  switch (stage) {
    case 'linking':
      return 'The image could not be linked. Check your connection and try again.'
    default:
      return 'The image is ready, but the VM could not be created. Try again.'
  }
}

interface VmCreationScreenProps {
  request: VmCreationRequest
  resource: VmCreationResource
}

/** VmCreationScreen shows the progress of the running VM creation. */
function VmCreationScreen({ request, resource }: VmCreationScreenProps) {
  const progress =
    resource.value ??
    ({
      stage: request.linkCdnImage ? 'linking' : 'creating',
    } satisfies VmCreationProgress)
  const error = resource.error
    ? creationFailureMessage(progress.stage)
    : undefined

  return (
    <VmCreationProgressScreen
      progress={progress}
      vmName={request.vmName}
      includesCdnLink={request.linkCdnImage}
      error={error}
      onRetry={error ? resource.retry : undefined}
    />
  )
}

/** WizardLoading shows the loading card until the wizard state arrives. */
function WizardLoading() {
  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
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

/** OperationErrorAlert shows why VM creation could not proceed. */
function OperationErrorAlert({ message }: { message: string }) {
  return (
    <div sx={stylexStyles2.rule3} role="alert">
      <div sx={stylexStyles2.rule4}>VM could not be created</div>
      <div sx={stylexStyles2.rule5}>{message}</div>
    </div>
  )
}

/**
 * VmV86WizardViewer is the wizard that creates a VM. Step 0 chooses the image:
 * one already in the Space, the image of the newest VM, or a published catalog
 * image. Step 1 names the VM and sets its memory. Finalize links the catalog
 * image when one was chosen, then creates the VM.
 */
export function VmV86WizardViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const { spaceWorldResource } = SpaceContainerContext.useContext()
  const rootResource = useRootResource()

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
  } = useWizardState({ objectInfo, worldState }, undefined)
  const cfg = useMemo(() => decodeConfig(configData), [configData])
  const [operationError, setOperationError] = useState('')

  useV86WizardStartupMarks(objectKey, !!wizardResource.value, state?.step)
  const { inSpaceImages, existingDefault, pending } =
    useV86WorldListings(spaceWorldResource)
  const { creationRequest, creationResource, startCreation } = useVmCreation()
  const { updateConfigDraft, persistConfig } = useV86ConfigWriter(
    wizardResource,
    handleConfigDataChange,
  )
  useV86Seed({
    ready: !!state,
    cfg,
    existingDefault,
    inSpaceImages,
    listingsPending: pending,
    persistConfig,
  })
  const cdnImageEntry = useDefaultCdnImage(rootResource, cfg, persistConfig)
  const cdnImage = selectedCdnImage(cfg, cdnImageEntry)
  const source = useV86SourceSelection({
    cfg,
    wizardResource,
    persistConfig,
    setOperationError,
  })
  const { creating, finalizeClick } = useV86Finalize({
    ready: !!state,
    cfg,
    localName,
    sessionPeerId,
    objectKey,
    cdnImage,
    persistDraftState,
    startCreation,
    setOperationError,
  })

  const handleMemoryChange = useCallback(
    (memoryMb: number) => {
      updateConfigDraft({ ...cfg, memoryMb })
    },
    [cfg, updateConfigDraft],
  )

  const handleCancelClick = useCallback(() => {
    void handleCancel()
  }, [handleCancel])

  if (creationRequest) {
    return (
      <VmCreationScreen request={creationRequest} resource={creationResource} />
    )
  }
  if (!state) return <WizardLoading />

  const step = state.step ?? 0
  return (
    <>
      <WizardShell
        title={
          <>
            <LuMonitor sx={stylexStyles.rule1} />
            New virtual machine
          </>
        }
        step={step}
        totalSteps={2}
        stepName={step === 0 ? 'Choose image' : 'Configure VM'}
        localName={localName}
        onUpdateName={handleUpdateName}
        onBack={() => void handleBack()}
        onCancel={handleCancelClick}
        nameLabel="VM name"
        namePlaceholder="e.g. debian-lab"
        nameStep={1}
        creating={creating}
        creatingLabel="Starting VM creation…"
        onFinalize={finalizeClick}
        canFinalize={!finalizeBlocker(cfg, cdnImage, sessionPeerId)}
        finalizeStep={1}
      >
        {step === 0 && (
          <V86SourceStep
            cfg={cfg}
            existingDefault={existingDefault}
            inSpaceImages={inSpaceImages}
            pending={pending}
            onSelectInSpace={source.selectInSpaceImage}
            onOpenCatalog={() => source.setCatalogOpen(true)}
          />
        )}
        {step === 1 && (
          <V86ConfigStep
            cfg={cfg}
            memoryMb={cfg.memoryMb || DEFAULT_V86_MEMORY_MB}
            onMemoryChange={handleMemoryChange}
            inSpaceImages={inSpaceImages}
            cdnImage={cdnImage}
            existingDefault={existingDefault}
          />
        )}
        {operationError && <OperationErrorAlert message={operationError} />}
      </WizardShell>
      <V86CatalogDialog
        open={source.catalogOpen}
        onOpenChange={source.setCatalogOpen}
        cdnId={cfg.cdnId ?? ''}
        selectedKey={cfg.cdnSourceObjectKey ?? ''}
        onSelect={source.pickCdnEntry}
      />
    </>
  )
}
