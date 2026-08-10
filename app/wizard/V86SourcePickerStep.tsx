import { LuCloud, LuHardDrive, LuMonitor, LuRefreshCcw } from 'react-icons/lu'

import type { V86Image } from '@s4wave/sdk/vm/v86.pb.js'
import {
  V86WizardConfig,
  V86WizardConfig_Source,
} from '@s4wave/sdk/vm/v86-wizard.pb.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

interface InSpaceV86ImageEntry {
  objectKey: string
  image: V86Image
}

interface ExistingVmInfo {
  imageKey: string
  name: string
}

export interface V86SourcePickerStepProps {
  cfg: V86WizardConfig
  existingDefault: ExistingVmInfo | undefined
  inSpaceImages: InSpaceV86ImageEntry[]
  onSelectInSpace: (imageKey: string) => void
  onOpenCdnPicker: () => void
  pending: boolean
}

export function V86SourcePickerStep({
  cfg,
  existingDefault,
  inSpaceImages,
  onSelectInSpace,
  onOpenCdnPicker,
  pending,
}: V86SourcePickerStepProps) {
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

function formatImageLabel(image: V86Image): string {
  const name = image.name || image.distro || 'V86Image'
  return image.version ? `${name} (${image.version})` : name
}
