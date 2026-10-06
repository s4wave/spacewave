import { useMemo, type ReactNode } from 'react'
import type { IconType } from 'react-icons'
import { LuFolderTree, LuPackage, LuTag, LuTerminal } from 'react-icons/lu'

import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { Manifest } from '@go/github.com/s4wave/spacewave/bldr/manifest/manifest.pb.js'
import type { BlockRef } from '@go/github.com/s4wave/spacewave/db/block/block.pb.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'

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
  icon: Icon,
  title,
  children,
}: ManifestSectionProps) {
  return (
    <section>
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
          <Icon className="size-3.5" />
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
    <div className="space-y-2">
      {meta?.manifestId && (
        <CopyableField label="Manifest ID" value={meta.manifestId} />
      )}
      {meta?.buildType && (
        <CopyableField label="Build Type" value={meta.buildType} />
      )}
      {meta?.platformId && (
        <CopyableField label="Platform" value={meta.platformId} />
      )}
      {meta?.rev !== undefined && (
        <CopyableField label="Rev" value={String(meta.rev)} />
      )}
      {meta?.description && (
        <CopyableField label="Description" value={meta.description} />
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
    <div className="bg-background-primary flex h-full w-full flex-col overflow-auto">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center border-b px-4">
        <div className="text-foreground flex items-center gap-2 text-sm font-semibold select-none">
          <LuPackage className="size-4" />
          <span className="tracking-tight">Manifest</span>
          {headerMeta && (
            <span className="text-foreground-alt/50 font-normal">
              {headerMeta}
            </span>
          )}
        </div>
      </div>
      <div className="flex-1 overflow-auto px-4 py-3">
        <div className="space-y-3">
          {isEmpty && (
            <InfoCard>
              <div className="text-foreground-alt/40 flex items-center gap-2 p-1 text-xs">
                <LuPackage className="size-3.5 shrink-0" />
                <span>No manifest data</span>
              </div>
            </InfoCard>
          )}
          {manifest?.entrypoint && (
            <ManifestSection icon={LuTerminal} title="Entrypoint">
              <CopyableField label="Path" value={manifest.entrypoint} />
            </ManifestSection>
          )}
          {hasStorage && (
            <ManifestSection icon={LuFolderTree} title="Storage">
              <div className="space-y-2">
                {distHash && (
                  <CopyableField label="Dist FS Ref" value={distHash} />
                )}
                {assetsHash && (
                  <CopyableField label="Assets FS Ref" value={assetsHash} />
                )}
              </div>
            </ManifestSection>
          )}
          {hasMeta && (
            <ManifestSection icon={LuTag} title="Metadata">
              <ManifestMetaFields meta={meta} />
            </ManifestSection>
          )}
        </div>
      </div>
    </div>
  )
}
