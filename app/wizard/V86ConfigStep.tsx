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
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
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
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'grid',
    gridTemplateColumns: {
      default: 'repeat(2, minmax(0, 1fr))',
      '@media (min-width: 40rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule4: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule6: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: themeLayout['--spacing'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
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
  sx,
  cfg,
  memoryMb,
  onMemoryChange,
  inSpaceImages,
  cdnImage,
  existingDefault,
}: V86ConfigStepProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const fromCatalog = cfg.source === V86WizardConfig_Source.LINK_FROM_CDN

  return (
    <div sx={[stylexStyles3.rule1, sx]}>
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
