import { LuCpu, LuHardDrive } from 'react-icons/lu'

import type { V86Image } from '@s4wave/sdk/vm/v86.pb.js'
import {
  V86WizardConfig,
  V86WizardConfig_Source,
} from '@s4wave/sdk/vm/v86-wizard.pb.js'
import { cn } from '@s4wave/web/style/utils.js'

const memoryOptions: readonly number[] = [64, 128, 256, 512, 1024]

interface ExistingVmInfo {
  imageKey: string
  name: string
}

export interface V86ConfigStepProps {
  cfg: V86WizardConfig
  memoryMb: number
  onMemoryChange: (memoryMb: number) => void
  selectedImage: V86Image | undefined
  selectedCdnImage: V86Image | undefined
  existingDefault: ExistingVmInfo | undefined
}

export function V86ConfigStep({
  cfg,
  memoryMb,
  onMemoryChange,
  selectedImage,
  selectedCdnImage,
  existingDefault,
}: V86ConfigStepProps) {
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
            {memoryOptions.map((memory) => (
              <button
                type="button"
                key={memory}
                className={cn(
                  'border-foreground/10 bg-background/20 text-foreground-alt hover:border-foreground/20 hover:bg-background/30 rounded-md border px-3 py-2 text-left text-xs transition-all duration-150 select-none',
                  memoryMb === memory &&
                    'border-brand/30 bg-brand/10 text-foreground',
                )}
                onClick={() => onMemoryChange(memory)}
              >
                {memory} MB
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

function formatImageLabel(image: V86Image): string {
  const name = image.name || image.distro || 'V86Image'
  return image.version ? `${name} (${image.version})` : name
}
