import {
  LuCloud,
  LuHardDrive,
  LuMonitor,
  LuRefreshCcw,
} from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import {
  V86WizardConfig_Source,
  type V86WizardConfig,
} from '@s4wave/sdk/vm/v86-wizard.pb.js'

import type { ExistingVm } from '../vm/v86-creation.js'
import { formatV86ImageLabel, type V86ImageEntry } from '../vm/v86-images.js'
import { V86ImageRow } from './V86ImageRow.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import { colors as themeColors } from '../../web/style/tokens.stylex.js'

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule3: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: { marginTop: `calc(${themeLayout['--spacing']} * 3)` },
})

// V86SourceStepProps describes the images a new VM can boot.
export interface V86SourceStepProps {
  cfg: V86WizardConfig
  // existingDefault is the newest VM in the Space that has an image.
  existingDefault: ExistingVm | undefined
  inSpaceImages: V86ImageEntry[]
  // pending is set while the Space's images and VMs are still loading.
  pending: boolean
  onSelectInSpace: (imageKey: string) => void
  onOpenCatalog: () => void
}

// V86SourceStep is the first wizard step: choose the image the VM boots,
// from this Space or from the published catalog.
export function V86SourceStep({
  sx,
  cfg,
  existingDefault,
  inSpaceImages,
  pending,
  onSelectInSpace,
  onOpenCatalog,
}: V86SourceStepProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const fromSpace = cfg.source === V86WizardConfig_Source.EXISTING_IN_SPACE
  const inheritKey = existingDefault?.imageKey ?? ''
  const inheritImage = inSpaceImages.find((e) => e.objectKey === inheritKey)
  const otherImages = inSpaceImages.filter((e) => e.objectKey !== inheritKey)
  const empty = !inheritKey && inSpaceImages.length === 0

  return (
    <section sx={[stylexStyles3.rule1, sx]}>
      <h3 sx={stylexStyles3.rule2}>
        <LuMonitor sx={stylexStyles2.rule1} />
        Choose a VM image
      </h3>

      {pending && empty && (
        <LoadingCard
          view={{
            state: 'active',
            title: 'Looking for VM images in this Space',
            detail: 'Reading images that are ready to use.',
          }}
        />
      )}

      {existingDefault && inheritKey && (
        <V86ImageRow
          icon={<LuRefreshCcw />}
          title={`Same image as ${existingDefault.name}`}
          detail={
            inheritImage
              ? formatV86ImageLabel(inheritImage.image)
              : 'The image the newest VM in this Space boots.'
          }
          selected={fromSpace && cfg.imageObjectKey === inheritKey}
          onSelect={() => onSelectInSpace(inheritKey)}
        />
      )}
      {otherImages.map((entry) => (
        <V86ImageRow
          key={entry.objectKey}
          icon={<LuHardDrive />}
          title={formatV86ImageLabel(entry.image)}
          detail={entry.image.distro || entry.objectKey}
          selected={fromSpace && cfg.imageObjectKey === entry.objectKey}
          onSelect={() => onSelectInSpace(entry.objectKey)}
        />
      ))}
      {!pending && !empty && (
        <V86ImageRow
          icon={<LuCloud />}
          title="Add image from catalog"
          detail="Use a published VM image. It downloads as the VM reads it."
          selected={cfg.source === V86WizardConfig_Source.LINK_FROM_CDN}
          onSelect={onOpenCatalog}
        />
      )}

      {!pending && empty && (
        <InfoCard
          icon={<LuHardDrive sx={stylexStyles2.rule2} />}
          title="No VM images in this Space"
        >
          <p sx={stylexStyles3.rule3}>
            Choose a published image from the catalog to continue.
          </p>
          <Button
            variant="brandOutline"
            size="toolbarWide"
            onClick={onOpenCatalog}
            sx={stylexStyles.rule1}
          >
            Browse image catalog
          </Button>
        </InfoCard>
      )}
    </section>
  )
}
