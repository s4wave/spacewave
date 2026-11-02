import { LuCpu, LuHardDrive } from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'
import type { V86Image } from '@s4wave/sdk/vm/v86.pb.js'
import {
  V86WizardConfig_Source,
  type V86WizardConfig,
} from '@s4wave/sdk/vm/v86-wizard.pb.js'

import type { ExistingVm } from '../vm/v86-creation.js'
import { formatV86ImageLabel, type V86ImageEntry } from '../vm/v86-images.js'

import * as stylex from '@stylexjs/stylex'

import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles = stylex.create({ rule1: { justifyContent: 'flex-start' } })

// memoryOptions lists the memory sizes, in MB, a new VM can choose.
const memoryOptions: readonly number[] = [64, 128, 256, 512, 1024]

// V86ConfigStepProps describes the VM being configured.
export interface V86ConfigStepProps {
  cfg: V86WizardConfig
  memoryMb: number
  onMemoryChange: (memoryMb: number) => void
  inSpaceImages: V86ImageEntry[]
  // cdnImage is the chosen catalog image once it has loaded.
  cdnImage: V86Image | undefined
  existingDefault: ExistingVm | undefined
}

// V86ConfigStep is the second wizard step: the VM's memory and a summary of
// the image it boots.
export function V86ConfigStep({
  cfg,
  memoryMb,
  onMemoryChange,
  inSpaceImages,
  cdnImage,
  existingDefault,
}: V86ConfigStepProps) {
  const fromCatalog = cfg.source === V86WizardConfig_Source.LINK_FROM_CDN

  return (
    <div className="border-foreground/6 bg-background-card/30 flex flex-col gap-3 rounded-lg border p-3.5">
      <section>
        <h3 className="text-foreground mb-2 flex items-center gap-1.5 text-xs font-medium select-none">
          <LuCpu sx={stylexStyles2.rule1} />
          Memory
        </h3>
        <div
          role="group"
          aria-label="Memory"
          className="grid grid-cols-2 gap-2 sm:grid-cols-3"
        >
          {memoryOptions.map((mb) => (
            <Button
              type="button"
              key={mb}
              aria-pressed={memoryMb === mb}
              variant={memoryMb === mb ? 'selectedRow' : 'unselectedRow'}
              size="sm"
              sx={stylexStyles.rule1}
              onClick={() => onMemoryChange(mb)}
            >
              {mb} MB
            </Button>
          ))}
        </div>
      </section>
      <section className="border-foreground/8 border-t pt-3">
        <h3 className="text-foreground mb-2 flex items-center gap-1.5 text-xs font-medium select-none">
          <LuHardDrive sx={stylexStyles2.rule1} />
          Image
        </h3>
        <div className="text-foreground text-sm font-medium">
          {describeImage(cfg, inSpaceImages, cdnImage, existingDefault)}
        </div>
        <p className="text-foreground-alt/70 mt-1 text-xs leading-relaxed">
          {fromCatalog
            ? 'The VM reads the published image as it runs.'
            : 'The VM uses an image already stored in this Space.'}
        </p>
      </section>
    </div>
  )
}

// describeImage names the image the config boots.
function describeImage(
  cfg: V86WizardConfig,
  inSpaceImages: V86ImageEntry[],
  cdnImage: V86Image | undefined,
  existingDefault: ExistingVm | undefined,
): string {
  if (cfg.source === V86WizardConfig_Source.LINK_FROM_CDN) {
    return cdnImage
      ? `From catalog: ${formatV86ImageLabel(cdnImage)}`
      : 'Loading the catalog image'
  }
  const entry = inSpaceImages.find((e) => e.objectKey === cfg.imageObjectKey)
  if (entry) return formatV86ImageLabel(entry.image)
  if (existingDefault && existingDefault.imageKey === cfg.imageObjectKey) {
    return `Same image as ${existingDefault.name}`
  }
  return cfg.imageObjectKey || 'Not selected'
}
