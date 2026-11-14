import { Root } from '@s4wave/sdk/root/root.js'
import {
  CreateV86ImageOp,
  CreateVmV86Op,
  SetV86ImageMetadataOp,
  VmV86,
  type V86Image,
} from '@s4wave/sdk/vm/v86.pb.js'
import {
  V86WizardConfig_Source,
  type V86WizardConfig,
} from '@s4wave/sdk/vm/v86-wizard.pb.js'
import {
  CREATE_V86_IMAGE_OP_ID,
  SET_V86_IMAGE_METADATA_OP_ID,
} from '@s4wave/sdk/vm/v86image.js'
import {
  CREATE_VM_V86_OP_ID,
  PRED_V86_IMAGE,
  VmV86TypeID,
} from '@s4wave/sdk/vm/vmv86.js'
import { iriToKey, keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { listObjectsWithType } from '@s4wave/sdk/world/types/types.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { buildObjectKey } from '../space/create-op-builders.js'
import { finishWizard, WizardGoneError } from '../wizard/wizard-finish.js'
import {
  DEFAULT_V86_MEMORY_MB,
  DEFAULT_V86_VGA_MEMORY_MB,
} from './v86-wizard-config.js'

// VmCreationStage is the step a VM creation is in.
export type VmCreationStage = 'linking' | 'creating' | 'ready'

// VmCreationProgress is one update of a running VM creation.
export interface VmCreationProgress {
  stage: VmCreationStage
}

// ExistingVm is a VM already in the Space and the image it boots.
export interface ExistingVm {
  objectKey: string
  name: string
  imageKey: string
  createdAt: Date | undefined
}

// VmCreationRequest is everything a VM creation needs, resolved up front.
export interface VmCreationRequest {
  // linkCdnImage is set when the Space image must first link the CDN image.
  linkCdnImage: boolean
  cdnId: string
  cdnSourceObjectKey: string
  cdnImage?: V86Image
  imageObjectKey: string
  vmKey: string
  vmName: string
  memoryMb: number
  vgaMemoryMb: number
  networking: boolean
  sessionPeerId: string
  wizardObjectKey: string
  root?: Root
}

// listExistingVms reads the VMs in world with their image, newest first.
export async function listExistingVms(
  world: EngineWorldState,
  signal: AbortSignal,
): Promise<ExistingVm[]> {
  const keys = await listObjectsWithType(world, VmV86TypeID, signal)
  const vms = await Promise.all(
    keys.map((key) => readExistingVm(world, key, signal)),
  )
  return vms
    .filter((vm): vm is ExistingVm => !!vm)
    .sort(
      (a, b) => (b.createdAt?.getTime() ?? 0) - (a.createdAt?.getTime() ?? 0),
    )
}

// readExistingVm reads the VM at objectKey, or undefined when it is missing
// or its record does not decode.
async function readExistingVm(
  world: EngineWorldState,
  objectKey: string,
  signal: AbortSignal,
): Promise<ExistingVm | undefined> {
  using obj = await world.getObject(objectKey, signal)
  if (!obj) return undefined
  using cursor = await obj.accessWorldState(undefined, signal)
  const resp = await cursor.unmarshal({ blockType: VmV86TypeID }, signal)
  if (!resp.found || !resp.data?.length) return undefined
  let vm: VmV86
  try {
    vm = VmV86.fromBinary(resp.data)
  } catch {
    return undefined
  }
  const edge = await world.lookupGraphQuads(
    keyToIRI(objectKey),
    PRED_V86_IMAGE,
    undefined,
    undefined,
    1,
    signal,
  )
  const target = edge.quads?.[0]?.obj
  return {
    objectKey,
    name: vm.name || objectKey,
    imageKey: target ? iriToKey(target) : '',
    createdAt: vm.createdAt,
  }
}

// hasImageChoice reports whether the config names an image to create from.
export function hasImageChoice(cfg: V86WizardConfig): boolean {
  if (!cfg.imageObjectKey) return false
  return (
    cfg.source !== V86WizardConfig_Source.LINK_FROM_CDN ||
    !!cfg.cdnSourceObjectKey
  )
}

// finalizeBlocker returns why the config cannot create a VM yet, or null when
// it can.
export function finalizeBlocker(
  cfg: V86WizardConfig,
  cdnImage: V86Image | undefined,
  sessionPeerId: string,
): string | null {
  if (!cfg.imageObjectKey) return 'Choose a VM image before creating the VM.'
  if (!hasImageChoice(cfg)) {
    return 'Choose an image from the catalog before creating the VM.'
  }
  if (cfg.source === V86WizardConfig_Source.LINK_FROM_CDN && !cdnImage) {
    return 'The catalog image is still loading. Try again in a moment.'
  }
  if (!sessionPeerId) {
    return 'VM creation is not ready in this session. Try again.'
  }
  return null
}

// BuildVmCreationRequestArgs is the wizard state a creation request needs.
export interface BuildVmCreationRequestArgs {
  cfg: V86WizardConfig
  vmName: string
  sessionPeerId: string
  wizardObjectKey: string
  root: Root | null | undefined
  cdnImage: V86Image | undefined
  spaceWorld: IWorldState
}

// buildVmCreationRequest resolves the creation request for the wizard config,
// including a free object key for the new VM.
export async function buildVmCreationRequest({
  cfg,
  vmName,
  sessionPeerId,
  wizardObjectKey,
  root,
  cdnImage,
  spaceWorld,
}: BuildVmCreationRequestArgs): Promise<VmCreationRequest> {
  const linkCdnImage = cfg.source === V86WizardConfig_Source.LINK_FROM_CDN
  if (linkCdnImage) {
    if (!root) throw new Error('root resource not ready')
    if (!cdnImage) throw new Error('CDN image metadata not loaded')
  }
  return {
    linkCdnImage,
    cdnId: cfg.cdnId ?? '',
    cdnSourceObjectKey: cfg.cdnSourceObjectKey ?? '',
    cdnImage,
    imageObjectKey: cfg.imageObjectKey ?? '',
    vmKey: await buildObjectKey(spaceWorld, 'vm/v86/', vmName),
    vmName,
    memoryMb: cfg.memoryMb || DEFAULT_V86_MEMORY_MB,
    vgaMemoryMb: cfg.vgaMemoryMb || DEFAULT_V86_VGA_MEMORY_MB,
    networking: cfg.networking ?? false,
    sessionPeerId,
    wizardObjectKey,
    root: root ?? undefined,
  }
}

// linkCdnV86Image writes the Space image as a link to the published CDN
// image. The VM reads each asset from the CDN on first use, so nothing is
// copied. Asset edges already on the Space image still take precedence.
async function linkCdnV86Image(
  world: EngineWorldState,
  request: VmCreationRequest,
  signal: AbortSignal,
) {
  const root = request.root
  if (!root) throw new Error('root resource not ready')
  if (!request.cdnImage) throw new Error('CDN image metadata not loaded')

  // Resolve the CDN Space the image is published in.
  using ownedRoot = root.resourceRef.createResource(
    root.resourceRef.resourceId,
    Root,
  )
  using cdn = (await ownedRoot.getCdn(request.cdnId, signal)).cdn
  const spaceId = await cdn.getCdnSpaceId(signal)

  // Create the linked image, or point an existing one at the chosen source.
  const image: V86Image = {
    ...request.cdnImage,
    cdnSource: { spaceId, objectKey: request.cdnSourceObjectKey },
  }
  using existing = await world.getObject(request.imageObjectKey, signal)
  if (existing) {
    await world.applyWorldOp(
      SET_V86_IMAGE_METADATA_OP_ID,
      SetV86ImageMetadataOp.toBinary({
        objectKey: request.imageObjectKey,
        image,
      }),
      request.sessionPeerId,
      signal,
    )
    return
  }
  await world.applyWorldOp(
    CREATE_V86_IMAGE_OP_ID,
    CreateV86ImageOp.toBinary({
      objectKey: request.imageObjectKey,
      image,
      timestamp: new Date(),
    }),
    request.sessionPeerId,
    signal,
  )
}

// runVmCreation links the CDN image when requested, then creates the VM and
// removes the wizard as one list, moving a Space index that names the wizard
// to the VM. The link runs first, outside the list, since it reads the CDN and
// reuses an image already linked. A failed create writes nothing, so a retry
// runs the stream again with the same key. It yields each stage as it starts.
export async function* runVmCreation(
  world: EngineWorldState,
  request: VmCreationRequest,
  signal: AbortSignal,
): AsyncGenerator<VmCreationProgress> {
  if (request.linkCdnImage) {
    yield { stage: 'linking' }
    await linkCdnV86Image(world, request, signal)
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
  try {
    await finishWizard(
      {
        spaceWorld: world,
        objectKey: request.wizardObjectKey,
        sessionPeerId: request.sessionPeerId,
      },
      {
        creates: [
          {
            kind: 'op',
            opId: CREATE_VM_V86_OP_ID,
            opData: CreateVmV86Op.toBinary(op),
          },
        ],
        target: { openKey: request.vmKey, indexPath: request.vmKey },
      },
      signal,
    )
  } catch (err) {
    // A rerun after this request's own commit landed finds the wizard gone
    // and the VM at its key: the creation is done, not refused.
    if (!(err instanceof WizardGoneError)) throw err
    using vm = await world.getObject(request.vmKey, signal)
    if (!vm) throw err
  }
  yield { stage: 'ready' }
}

/**
 * creationRefusedAsGone reports whether a creation that failed did so because
 * the wizard is gone: refused as gone, or failed while another writer removed
 * it, so Retry could never succeed.
 */
export function creationRefusedAsGone(
  error: Error | null | undefined,
  wizardGone: boolean,
): boolean {
  if (!error) return false
  return wizardGone || error instanceof WizardGoneError
}
