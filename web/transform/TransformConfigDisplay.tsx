import { useMemo } from 'react'
import {
  LuHardDrive,
  LuLock,
  LuArchive,
  LuShieldCheck,
  LuUsers,
  LuLayers,
} from 'react-icons/lu'

import { Config as BlockEncCfg } from '@go/github.com/s4wave/spacewave/db/block/transform/blockenc/blockenc.pb.js'
import { BlockEnc } from '@go/github.com/s4wave/spacewave/db/util/blockenc/blockenc.pb.js'
import {
  Config as LZ4Cfg,
  BlockSize,
} from '@go/github.com/s4wave/spacewave/db/block/transform/lz4/lz4.pb.js'
import { Config as S2Cfg } from '@go/github.com/s4wave/spacewave/db/block/transform/s2/s2.pb.js'
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

// formatBytes formats a byte count as a human-readable string.
export function formatBytes(bytes: bigint | number): string {
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
const STEP_S2 = 'hydra/transform/s2'
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
  let name = 'XChaCha20-Poly1305'
  if (config && config.length > 0) {
    try {
      const decoded = BlockEncCfg.fromBinary(config)
      switch (decoded.blockEnc) {
        case BlockEnc.BlockEnc_SECRET_BOX:
          name = 'SecretBox'
          break
        case BlockEnc.BlockEnc_NONE:
          name = 'None'
          break
      }
    } catch {
      // fall back to default name
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

function decodeS2(config?: Uint8Array): { detail: string; tooltip: string } {
  let mode = 'Fast'
  if (config && config.length > 0) {
    try {
      const decoded = S2Cfg.fromBinary(config)
      mode =
        decoded.best ? 'Best'
        : decoded.better ? 'Better'
        : 'Fast'
    } catch {
      // fall back to defaults
    }
  }
  return {
    detail: `S2 ${mode}`,
    tooltip:
      `Data is compressed with S2 (${mode} mode) before storage to reduce size and transfer time. ` +
      'S2 is optimized for speed with good compression ratios.',
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
    case STEP_S2: {
      const { detail, tooltip } = decodeS2(step.config)
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
  const steps = useMemo(() => (info.steps ?? []).map(parseStep), [info.steps])
  const encryptionStep = steps.find((s) => s.isEncryption)
  const contentSteps = steps.filter((s) => !s.isEncryption)
  const hasEncryption = encryptionStep != null
  const grantCount = info.grantCount ?? 0
  const storageBytes = info.storageBytes ?? 0n

  return (
    <div
      className={cn(
        'divide-foreground/6 divide-y overflow-hidden rounded-lg border backdrop-blur-sm',
        hasEncryption ?
          'border-brand/12 bg-background-card/40'
        : 'border-foreground/6 bg-background-card/30',
      )}
      style={{
        boxShadow:
          hasEncryption ?
            '0 4px 20px rgba(0,0,0,0.25), 0 0 40px rgba(200,80,60,0.04)'
          : '0 2px 8px rgba(0,0,0,0.15)',
      }}
    >
      {contentSteps.map((step, i) => {
        const Icon = step.icon
        return (
          <Tooltip key={i}>
            <TooltipTrigger asChild>
              <div className="flex cursor-default items-center gap-3 px-4 py-2">
                <div
                  className={cn(
                    'bg-foreground/5 flex h-5 w-5 shrink-0 items-center justify-center rounded-md',
                  )}
                >
                  <Icon className="text-foreground-alt h-3 w-3" />
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-foreground-alt text-xs font-medium">
                    {step.label}
                  </span>
                  <span className="text-foreground-alt/50 text-[0.6rem]">
                    {step.detail}
                  </span>
                </div>
              </div>
            </TooltipTrigger>
            <TooltipContent side="top" className="max-w-64">
              {step.tooltip}
            </TooltipContent>
          </Tooltip>
        )
      })}

      {storageBytes > 0n && (
        <Tooltip>
          <TooltipTrigger asChild>
            <div className="flex cursor-default items-center gap-3 px-4 py-2">
              <div className="bg-foreground/5 flex h-5 w-5 shrink-0 items-center justify-center rounded-md">
                <LuHardDrive className="text-foreground-alt h-3 w-3" />
              </div>
              <div className="flex items-center gap-2">
                <span className="text-foreground-alt text-xs font-medium">
                  Storage
                </span>
                <span className="text-foreground-alt/50 text-[0.6rem]">
                  {formatBytes(storageBytes)}
                </span>
              </div>
            </div>
          </TooltipTrigger>
          <TooltipContent side="top" className="max-w-64">
            Total storage used by this space on disk.
          </TooltipContent>
        </Tooltip>
      )}

      <Tooltip>
        <TooltipTrigger asChild>
          <div
            className={cn(
              'flex cursor-default items-center justify-between gap-3 px-4 py-1.5',
              hasEncryption ?
                'bg-brand/6 border-brand/12 text-brand/80'
              : 'bg-foreground/3 text-foreground-alt/60',
            )}
          >
            <div className="flex min-w-0 items-center gap-2">
              <LuLock
                className={cn(
                  'h-3 w-3 shrink-0',
                  hasEncryption ? 'text-brand/80' : 'text-foreground-alt/40',
                )}
              />
              <span className="text-[0.55rem] font-medium tracking-[0.14em] uppercase">
                Encryption
              </span>
              <span
                className={cn(
                  'truncate text-[0.65rem]',
                  hasEncryption ? 'text-brand/90' : 'text-foreground-alt/60',
                )}
              >
                {encryptionStep?.detail ?? 'Not encrypted'}
              </span>
            </div>
            {hasEncryption && grantCount > 0 && (
              <div className="flex shrink-0 items-center gap-1 text-[0.55rem] font-medium">
                <LuUsers className="h-2.5 w-2.5" />
                {grantCount} {grantCount === 1 ? 'key' : 'keys'}
              </div>
            )}
          </div>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-64">
          {encryptionStep?.tooltip ??
            'Data is not encrypted before storage. Anyone with access to the storage can read it.'}
        </TooltipContent>
      </Tooltip>
    </div>
  )
}
