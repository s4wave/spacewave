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

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule3: {
    display: 'grid',
    gridTemplateColumns: {
      default: 'repeat(2, minmax(0, 1fr))',
      '@media (min-width: 40rem)': 'repeat(3, minmax(0, 1fr))',
    },
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule5: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule6: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
})

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
    <div sx={stylexStyles3.rule1}>
      <section>
        <h3 sx={stylexStyles3.rule2}>
          <LuCpu sx={stylexStyles2.rule1} />
          Memory
        </h3>
        <div role="group" aria-label="Memory" sx={stylexStyles3.rule3}>
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
      <section sx={stylexStyles3.rule4}>
        <h3 sx={stylexStyles3.rule2}>
          <LuHardDrive sx={stylexStyles2.rule1} />
          Image
        </h3>
        <div sx={stylexStyles3.rule5}>
          {describeImage(cfg, inSpaceImages, cdnImage, existingDefault)}
        </div>
        <p sx={stylexStyles3.rule6}>
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
