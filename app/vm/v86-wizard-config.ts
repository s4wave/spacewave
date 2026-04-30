import type { VmImage } from '@s4wave/sdk/vm/v86.pb.js'
import type { V86WizardConfig } from '@s4wave/sdk/vm/v86-wizard.pb.js'
import { V86WizardConfig_Source } from '@s4wave/sdk/vm/v86-wizard.pb.js'

import { buildWizardObjectKey } from '@s4wave/app/space/create-op-builders.js'

export interface ExistingVmImageSource {
  imageKey: string
}

export interface InSpaceVmImageSource {
  objectKey: string
}

export const V86_WIZARD_TYPE_ID = 'wizard/v86'
export const V86_WIZARD_TARGET_TYPE_ID = 'v86'
export const V86_WIZARD_TARGET_KEY_PREFIX = 'vm/v86/'

export const V86_USER_IMAGE_OBJECT_KEY = 'vm-image/default'

export const DEFAULT_V86_MEMORY_MB = 256
export const DEFAULT_V86_VGA_MEMORY_MB = 8

export const V86_DEFAULT_IMAGE_PLATFORM = 'v86'
export const V86_DEFAULT_IMAGE_TAG = 'default'

export function buildV86QuickstartWizardConfig(): V86WizardConfig {
  return {
    name: '',
    memoryMb: DEFAULT_V86_MEMORY_MB,
    vgaMemoryMb: DEFAULT_V86_VGA_MEMORY_MB,
    networking: false,
    imageObjectKey: V86_USER_IMAGE_OBJECT_KEY,
    source: V86WizardConfig_Source.COPY_FROM_CDN,
    cdnSourceObjectKey: '',
    cdnId: '',
  }
}

export function buildV86QuickstartWizardKey(now: Date): string {
  return buildWizardObjectKey('V86 VM ' + now.getTime().toString(36))
}

export function seedV86WizardConfig(
  cfg: V86WizardConfig,
  existingDefault: ExistingVmImageSource | undefined,
  inSpaceImages: InSpaceVmImageSource[],
): V86WizardConfig {
  if (
    (cfg.source ?? V86WizardConfig_Source.SOURCE_UNSPECIFIED) !==
    V86WizardConfig_Source.SOURCE_UNSPECIFIED
  ) {
    return cfg
  }

  const next: V86WizardConfig = { ...cfg }
  if (existingDefault?.imageKey) {
    next.source = V86WizardConfig_Source.EXISTING_IN_SPACE
    next.imageObjectKey = existingDefault.imageKey
  } else if (inSpaceImages[0]) {
    next.source = V86WizardConfig_Source.EXISTING_IN_SPACE
    next.imageObjectKey = inSpaceImages[0].objectKey
  } else {
    next.source = V86WizardConfig_Source.COPY_FROM_CDN
    next.imageObjectKey = V86_USER_IMAGE_OBJECT_KEY
  }
  if (!next.memoryMb) next.memoryMb = DEFAULT_V86_MEMORY_MB
  if (!next.vgaMemoryMb) next.vgaMemoryMb = DEFAULT_V86_VGA_MEMORY_MB
  return next
}

export function compareVmImageNewestFirst(
  a: { image: VmImage; objectKey: string },
  b: { image: VmImage; objectKey: string },
): number {
  const ta = a.image.createdAt?.getTime() ?? 0
  const tb = b.image.createdAt?.getTime() ?? 0
  if (ta !== tb) return tb - ta
  const va = a.image.version ?? ''
  const vb = b.image.version ?? ''
  if (va !== vb) return vb.localeCompare(va)
  return a.objectKey.localeCompare(b.objectKey)
}

export function isDefaultV86VmImage(image: VmImage): boolean {
  if (image.platform !== V86_DEFAULT_IMAGE_PLATFORM) {
    return false
  }
  return (image.tags ?? []).includes(V86_DEFAULT_IMAGE_TAG)
}
