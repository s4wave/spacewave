import { useMemo, type ReactNode } from 'react'
import type { IconType } from '@s4wave/web/images/icons.js'
import {
  LuFolderTree,
  LuPackage,
  LuTag,
  LuTerminal,
} from '@s4wave/web/images/icons.js'

import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { Manifest } from '@go/github.com/s4wave/spacewave/bldr/manifest/manifest.pb.js'
import type { BlockRef } from '@go/github.com/s4wave/spacewave/db/block/block.pb.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule2: {
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
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    overflow: 'auto',
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule4: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule6: {
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule7: {
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule8: {
    flex: 1,
    overflow: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule9: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    padding: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
})

export const ManifestTypeID = 'bldr/manifest'

// formatBlockRefHash formats a BlockRef hash as a truncated hex string.
function formatBlockRefHash(ref: BlockRef | undefined): string {
  const hash = ref?.hash?.hash
  if (!hash?.length) return ''
  const hex = Array.from(hash, (b) => b.toString(16).padStart(2, '0')).join('')
  if (hex.length <= 16) return hex
  return hex.slice(0, 8) + '...' + hex.slice(-8)
}

// manifestHeaderMeta summarizes the platform and revision for the header.
function manifestHeaderMeta(meta: Manifest['meta']): string {
  return [meta?.platformId, meta?.rev !== undefined ? `rev ${meta.rev}` : null]
    .filter(Boolean)
    .join(' · ')
}

// hasManifestMeta reports whether the manifest carries any identity field.
function hasManifestMeta(meta: Manifest['meta']): boolean {
  return !!(
    meta?.manifestId ||
    meta?.buildType ||
    meta?.platformId ||
    meta?.rev !== undefined ||
    meta?.description
  )
}

interface ManifestSectionProps {
  icon: IconType
  title: string
  children: ReactNode
}

/** ManifestSection renders a titled info card of manifest fields. */
function ManifestSection({
  sx,
  icon: Icon,
  title,
  children,
}: ManifestSectionProps & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <section sx={sx}>
      <div sx={stylexStyles3.rule1}>
        <h2 sx={stylexStyles3.rule2}>
          <Icon sx={stylexStyles2.rule1} />
          {title}
        </h2>
      </div>
      <InfoCard>{children}</InfoCard>
    </section>
  )
}

interface ManifestMetaFieldsProps {
  meta: Manifest['meta']
}

/** ManifestMetaFields lists the copyable identity fields of a manifest. */
function ManifestMetaFields({ meta }: ManifestMetaFieldsProps) {
  return (
    <div sx={false}>
      {meta?.manifestId && (
        <CopyableField
          sx={nativeChildSpacing.s2}
          label="Manifest ID"
          value={meta.manifestId}
        />
      )}
      {meta?.buildType && (
        <CopyableField
          sx={nativeChildSpacing.s2}
          label="Build Type"
          value={meta.buildType}
        />
      )}
      {meta?.platformId && (
        <CopyableField
          sx={nativeChildSpacing.s2}
          label="Platform"
          value={meta.platformId}
        />
      )}
      {meta?.rev !== undefined && (
        <CopyableField
          sx={nativeChildSpacing.s2}
          label="Rev"
          value={String(meta.rev)}
        />
      )}
      {meta?.description && (
        <CopyableField
          sx={nativeChildSpacing.s2}
          label="Description"
          value={meta.description}
        />
      )}
    </div>
  )
}

// ManifestViewer displays a bldr Manifest world object.
export function ManifestViewer({
  objectInfo: _objectInfo,
  objectState,
}: ObjectViewerComponentProps) {
  const manifest = useForgeBlockData(objectState, ManifestTypeID, Manifest)
  const meta = manifest?.meta

  const distHash = useMemo(
    () => formatBlockRefHash(manifest?.distFsRef),
    [manifest?.distFsRef],
  )
  const assetsHash = useMemo(
    () => formatBlockRefHash(manifest?.assetsFsRef),
    [manifest?.assetsFsRef],
  )

  const headerMeta = manifestHeaderMeta(meta)
  const hasMeta = hasManifestMeta(meta)
  const hasEntrypoint = !!manifest?.entrypoint
  const hasStorage = !!(distHash || assetsHash)
  const isEmpty = !hasMeta && !hasEntrypoint && !hasStorage

  return (
    <div sx={stylexStyles3.rule3}>
      <div sx={stylexStyles3.rule4}>
        <div sx={stylexStyles3.rule5}>
          <LuPackage sx={stylexStyles.rule1} />
          <span sx={stylexStyles3.rule6}>Manifest</span>
          {headerMeta && <span sx={stylexStyles3.rule7}>{headerMeta}</span>}
        </div>
      </div>
      <div sx={stylexStyles3.rule8}>
        <div sx={false}>
          {isEmpty && (
            <InfoCard sx={nativeChildSpacingNext.s3}>
              <div sx={stylexStyles3.rule9}>
                <LuPackage sx={stylexStyles.rule2} />
                <span>No manifest data</span>
              </div>
            </InfoCard>
          )}
          {manifest?.entrypoint && (
            <ManifestSection
              sx={nativeChildSpacingNext.s3}
              icon={LuTerminal}
              title="Entrypoint"
            >
              <CopyableField label="Path" value={manifest.entrypoint} />
            </ManifestSection>
          )}
          {hasStorage && (
            <ManifestSection
              sx={nativeChildSpacingNext.s3}
              icon={LuFolderTree}
              title="Storage"
            >
              <div sx={false}>
                {distHash && (
                  <CopyableField
                    sx={nativeChildSpacing.s2}
                    label="Dist FS Ref"
                    value={distHash}
                  />
                )}
                {assetsHash && (
                  <CopyableField
                    sx={nativeChildSpacing.s2}
                    label="Assets FS Ref"
                    value={assetsHash}
                  />
                )}
              </div>
            </ManifestSection>
          )}
          {hasMeta && (
            <ManifestSection
              sx={nativeChildSpacingNext.s3}
              icon={LuTag}
              title="Metadata"
            >
              <ManifestMetaFields meta={meta} />
            </ManifestSection>
          )}
        </div>
      </div>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
