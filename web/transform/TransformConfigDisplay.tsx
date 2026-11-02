import { useMemo } from 'react'
import {
  LuHardDrive,
  LuLock,
  LuArchive,
  LuShieldCheck,
  LuUsers,
  LuLayers,
} from '@s4wave/web/images/icons.js'

import { Config as BlockEncCfg } from '@go/github.com/s4wave/spacewave/db/block/transform/blockenc/blockenc.pb.js'
import { BlockEnc } from '@go/github.com/s4wave/spacewave/db/util/blockenc/blockenc.pb.js'
import {
  Config as LZ4Cfg,
  BlockSize,
} from '@go/github.com/s4wave/spacewave/db/block/transform/lz4/lz4.pb.js'
import { Config as GzipCfg } from '@go/github.com/s4wave/spacewave/db/block/transform/gzip/gzip.pb.js'
import {
  Config as ChksumCfg,
  ChksumType,
} from '@go/github.com/s4wave/spacewave/db/block/transform/chksum/chksum.pb.js'
import type { StepConfig } from '@go/github.com/s4wave/spacewave/db/block/transform/transform.pb.js'
import type { TransformInfo } from '@s4wave/sdk/space/space.pb.js'

import { cn } from '@s4wave/web/style/utils.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../style/tokens.stylex.js'

import {
  layout as themeLayout2,
  radii as themeRadii,
  colors as themeColors,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    cursor: 'default',
    alignItems: 'center',
    gap: `calc(${themeLayout2['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout2['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout2['--spacing']} * 2)`,
  },
  rule2: {
    display: 'flex',
    width: `calc(${themeLayout2['--spacing']} * 5)`,
    height: `calc(${themeLayout2['--spacing']} * 5)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule3: {
    width: `calc(${themeLayout2['--spacing']} * 3)`,
    height: `calc(${themeLayout2['--spacing']} * 3)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout2['--spacing']} * 2)`,
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
  },
  rule6: {
    fontSize: '.6rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule7: {
    display: 'flex',
    cursor: 'default',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout2['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout2['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout2['--spacing']} * 1.5)`,
  },
  rule8: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 12%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 6%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
  },
  rule9: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 3%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule10: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout2['--spacing']} * 2)`,
  },
  rule11: {
    width: `calc(${themeLayout2['--spacing']} * 3)`,
    height: `calc(${themeLayout2['--spacing']} * 3)`,
    flexShrink: 0,
  },
  rule12: {
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
  },
  rule13: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule14: {
    fontSize: '.55rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: '.14em',
    textTransform: 'uppercase',
  },
  rule15: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: '.65rem',
    lineHeight: 1.5,
  },
  rule16: {
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 90%, transparent)`,
  },
  rule17: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule18: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: themeLayout2['--spacing'],
    fontSize: '.55rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule19: {
    width: `calc(${themeLayout2['--spacing']} * 2.5)`,
    height: `calc(${themeLayout2['--spacing']} * 2.5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: { maxWidth: `calc(${themeLayout['--spacing']} * 64)` },
})

// formatBytes formats a byte count as a human-readable string.
export function formatBytes(bytes: bigint | number): string {
  // Choose a binary unit for the supplied byte quantity.
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = typeof bytes === 'bigint' ? Number(bytes) : bytes
  let idx = 0
  while (size >= 1024 && idx < units.length - 1) {
    size /= 1024
    idx++
  }
  if (idx === 0) return `${size} ${units[idx]}`
  return `${size.toFixed(1)} ${units[idx]}`
}

// --- Step decoding ---

const STEP_BLOCKENC = 'hydra/transform/blockenc'
const STEP_LZ4 = 'hydra/transform/lz4'
const STEP_GZIP = 'hydra/transform/gzip'
const STEP_CHKSUM = 'hydra/transform/chksum'

interface StepInfo {
  icon: typeof LuLock
  label: string
  detail: string
  tooltip: string
  isEncryption: boolean
}

function decodeBlockEnc(config?: Uint8Array): {
  detail: string
  tooltip: string
} {
  let name = 'Unknown'
  if (config && config.length > 0) {
    try {
      const decoded = BlockEncCfg.fromBinary(config)
      switch (decoded.blockEnc) {
        case BlockEnc.BlockEnc_AES_256_GCM:
          name = 'AES-256-GCM'
          break
        case BlockEnc.BlockEnc_XCHACHA20_POLY1305:
          name = 'XChaCha20-Poly1305'
          break
        case BlockEnc.BlockEnc_SECRET_BOX:
          name = 'SecretBox'
          break
        case BlockEnc.BlockEnc_NONE:
          name = 'None'
          break
      }
    } catch {
      // fall back to unknown when the redacted config cannot be decoded
    }
  }
  return {
    detail: name,
    tooltip:
      'All data is encrypted on your device before storage. ' +
      `Using ${name} authenticated encryption. ` +
      'Only participants with the encryption key can read it.',
  }
}

function decodeLZ4(config?: Uint8Array): { detail: string; tooltip: string } {
  // Use LZ4 defaults unless its optional encoded configuration supplies supported values.
  let blockSize = '4 MB'
  let level = 0
  if (config && config.length > 0) {
    try {
      const decoded = LZ4Cfg.fromBinary(config)
      level = decoded.compressionLevel ?? 0
      switch (decoded.blockSize) {
        case BlockSize.BlockSize_64KB:
          blockSize = '64 KB'
          break
        case BlockSize.BlockSize_256KB:
          blockSize = '256 KB'
          break
        case BlockSize.BlockSize_1MB:
          blockSize = '1 MB'
          break
      }
    } catch {
      // fall back to defaults
    }
  }
  const parts = [`Block: ${blockSize}`]
  if (level > 0) parts.push(`Level: ${level}`)
  return {
    detail: `LZ4 (${parts.join(', ')})`,
    tooltip:
      'Data is compressed with LZ4 before storage to reduce size and transfer time. ' +
      `Block size: ${blockSize}.` +
      (level > 0 ? ` Compression level: ${level}.` : ''),
  }
}

function decodeGzip(config?: Uint8Array): { detail: string; tooltip: string } {
  let mode = 'Default'
  if (config && config.length > 0) {
    try {
      const decoded = GzipCfg.fromBinary(config)
      mode =
        decoded.compressionLevel === undefined || decoded.compressionLevel === 0
          ? 'Default'
          : `Level ${decoded.compressionLevel}`
    } catch {
      // fall back to defaults
    }
  }
  return {
    detail: `Gzip ${mode}`,
    tooltip:
      `Data is compressed with Gzip (${mode} mode) before storage to reduce size and transfer time. ` +
      'Gzip is optimized for speed with good compression ratios.',
  }
}

function decodeChksum(config?: Uint8Array): {
  detail: string
  tooltip: string
} {
  let name = 'CRC64'
  if (config && config.length > 0) {
    try {
      const decoded = ChksumCfg.fromBinary(config)
      if (decoded.chksumType === ChksumType.ChksumType_CRC32) name = 'CRC32'
    } catch {
      // fall back to default name
    }
  }
  return {
    detail: name,
    tooltip: `A ${name} checksum is computed for each block to detect data corruption during storage or transfer.`,
  }
}

function parseStep(step: StepConfig): StepInfo {
  switch (step.id) {
    case STEP_BLOCKENC: {
      const { detail, tooltip } = decodeBlockEnc(step.config)
      return {
        icon: LuLock,
        label: 'Encryption',
        detail,
        tooltip,
        isEncryption: true,
      }
    }
    case STEP_LZ4: {
      const { detail, tooltip } = decodeLZ4(step.config)
      return {
        icon: LuArchive,
        label: 'Compression',
        detail,
        tooltip,
        isEncryption: false,
      }
    }
    case STEP_GZIP: {
      const { detail, tooltip } = decodeGzip(step.config)
      return {
        icon: LuArchive,
        label: 'Compression',
        detail,
        tooltip,
        isEncryption: false,
      }
    }
    case STEP_CHKSUM: {
      const { detail, tooltip } = decodeChksum(step.config)
      return {
        icon: LuShieldCheck,
        label: 'Integrity',
        detail,
        tooltip,
        isEncryption: false,
      }
    }
    default:
      return {
        icon: LuLayers,
        label: step.id ?? 'Unknown',
        detail: step.config ? `${step.config.length} bytes` : '',
        tooltip: `Transform step: ${step.id ?? 'unknown'}`,
        isEncryption: false,
      }
  }
}

// --- Component ---

export interface TransformConfigDisplayProps {
  info: TransformInfo
}

// TransformConfigDisplay renders transform pipeline info as an integrated card with rows.
export function TransformConfigDisplay({ info }: TransformConfigDisplayProps) {
  // Separate encryption from content transforms and resolve their storage summary.
  const steps = useMemo(() => (info.steps ?? []).map(parseStep), [info.steps])
  const encryptionStep = steps.find((s) => s.isEncryption)
  const contentSteps = steps.filter((s) => !s.isEncryption)
  const hasEncryption = encryptionStep != null
  const grantCount = info.grantCount ?? 0
  const storageBytes = info.storageBytes ?? 0n

  return (
    <div
      className={
        /* TODO(stylex): global or unknown class: transform-config-shadow-encryption */ cn(
          hasEncryption
            ? 'transform-config-shadow-encryption'
            : 'transform-config-shadow-default',
          'divide-foreground/6 divide-y overflow-hidden rounded-lg border',
          hasEncryption
            ? 'border-brand/12 bg-background-card/40'
            : 'border-foreground/6 bg-background-card/30',
        )
      }
    >
      {contentSteps.map((step) => {
        const Icon = step.icon
        return (
          <Tooltip key={`${step.label}-${step.detail}`}>
            <TooltipTrigger asChild>
              <div sx={stylexStyles2.rule1}>
                <div sx={[stylexStyles2.rule2]}>
                  <Icon sx={stylexStyles2.rule3} />
                </div>
                <div sx={stylexStyles2.rule4}>
                  <span sx={stylexStyles2.rule5}>{step.label}</span>
                  <span sx={stylexStyles2.rule6}>{step.detail}</span>
                </div>
              </div>
            </TooltipTrigger>
            <TooltipContent side="top" sx={stylexStyles.rule1}>
              {step.tooltip}
            </TooltipContent>
          </Tooltip>
        )
      })}

      {storageBytes > 0n && (
        <Tooltip>
          <TooltipTrigger asChild>
            <div sx={stylexStyles2.rule1}>
              <div sx={stylexStyles2.rule2}>
                <LuHardDrive sx={stylexStyles2.rule3} />
              </div>
              <div sx={stylexStyles2.rule4}>
                <span sx={stylexStyles2.rule5}>Storage</span>
                <span sx={stylexStyles2.rule6}>
                  {formatBytes(storageBytes)}
                </span>
              </div>
            </div>
          </TooltipTrigger>
          <TooltipContent side="top" sx={stylexStyles.rule1}>
            Total storage used by this space on disk.
          </TooltipContent>
        </Tooltip>
      )}

      <Tooltip>
        <TooltipTrigger asChild>
          <div
            sx={[
              stylexStyles2.rule7,
              hasEncryption ? stylexStyles2.rule8 : stylexStyles2.rule9,
            ]}
          >
            <div sx={stylexStyles2.rule10}>
              <LuLock
                sx={[
                  stylexStyles2.rule11,
                  hasEncryption ? stylexStyles2.rule12 : stylexStyles2.rule13,
                ]}
              />
              <span sx={stylexStyles2.rule14}>Encryption</span>
              <span
                sx={[
                  stylexStyles2.rule15,
                  hasEncryption ? stylexStyles2.rule16 : stylexStyles2.rule17,
                ]}
              >
                {encryptionStep?.detail ?? 'Not encrypted'}
              </span>
            </div>
            {hasEncryption && grantCount > 0 && (
              <div sx={stylexStyles2.rule18}>
                <LuUsers sx={stylexStyles2.rule19} />
                {grantCount} {grantCount === 1 ? 'key' : 'keys'}
              </div>
            )}
          </div>
        </TooltipTrigger>
        <TooltipContent side="top" sx={stylexStyles.rule1}>
          {encryptionStep?.tooltip ??
            'Data is not encrypted before storage. Anyone with access to the storage can read it.'}
        </TooltipContent>
      </Tooltip>
    </div>
  )
}
