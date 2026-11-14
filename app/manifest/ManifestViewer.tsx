import { useMemo, type ReactNode } from 'react'
import type { IconType } from 'react-icons'
import { LuFolderTree, LuPackage, LuTag, LuTerminal } from 'react-icons/lu'

import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { FactList, FactRow } from '@s4wave/web/ui/FactList.js'
import { Manifest } from '@go/github.com/s4wave/spacewave/bldr/manifest/manifest.pb.js'
import type { BlockRef } from '@go/github.com/s4wave/spacewave/db/block/block.pb.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'
import { ViewerBar } from '@s4wave/web/object/ViewerBar.js'

export const ManifestTypeID = 'bldr/manifest'

// formatBlockRefHash formats a BlockRef hash as its whole hex string, which
// the fact row shortens for display and copies whole; a missing hash formats
// as empty.
function formatBlockRefHash(ref: BlockRef | undefined): string {
  const hash = ref?.hash?.hash
  if (!hash?.length) return ''
  return Array.from(hash, (b) => b.toString(16).padStart(2, '0')).join('')
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
        <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
          <Icon className="size-3.5" />
          {title}
        </h3>
      </div>
      <InfoCard>{children}</InfoCard>
    </section>
  )
}

interface ManifestMetaFieldsProps {
  meta: Manifest['meta']
}

/**
 * ManifestMetaFields lists the identity facts of a manifest, with its
 * description as prose below them.
 */
function ManifestMetaFields({ meta }: ManifestMetaFieldsProps) {
  return (
    <>
      <FactList>
        {meta?.manifestId && (
          <FactRow label="Manifest ID" value={meta.manifestId} />
        )}
        {meta?.buildType && (
          <FactRow kind="text" label="Build Type" value={meta.buildType} />
        )}
        {meta?.platformId && (
          <FactRow label="Platform" value={meta.platformId} />
        )}
        {meta?.rev !== undefined && (
          <FactRow kind="text" label="Rev" value={String(meta.rev)} />
        )}
      </FactList>
      {meta?.description && (
        <p className="text-foreground mt-3 text-sm break-words whitespace-pre-wrap">
          {meta.description}
        </p>
      )}
    </>
  )
}

// ManifestViewer displays a bldr Manifest world object.
export function ManifestViewer({
  objectInfo: _objectInfo,
  objectState,
}: ObjectViewerComponentProps) {
  // Read the manifest block and its metadata.
  const manifest = useForgeBlockData(objectState, ManifestTypeID, Manifest)
  const meta = manifest?.meta

  // Format the storage refs once per ref.
  const distHash = useMemo(
    () => formatBlockRefHash(manifest?.distFsRef),
    [manifest?.distFsRef],
  )
  const assetsHash = useMemo(
    () => formatBlockRefHash(manifest?.assetsFsRef),
    [manifest?.assetsFsRef],
  )

  // Decide which sections have content.
  const headerMeta = manifestHeaderMeta(meta)
  const hasMeta = hasManifestMeta(meta)
  const hasEntrypoint = !!manifest?.entrypoint
  const hasStorage = !!(distHash || assetsHash)
  const isEmpty = !hasMeta && !hasEntrypoint && !hasStorage

  return (
    <div className="bg-background-primary flex h-full w-full flex-col overflow-auto">
      <ViewerBar
        icon={<LuPackage />}
        title="Manifest"
        description={headerMeta || undefined}
      />
      <div className="viewer-inset flex-1 overflow-auto">
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
              <FactRow label="Path" value={manifest.entrypoint} />
            </ManifestSection>
          )}
          {hasStorage && (
            <ManifestSection icon={LuFolderTree} title="Storage">
              <FactList>
                {distHash && <FactRow label="Dist FS Ref" value={distHash} />}
                {assetsHash && (
                  <FactRow label="Assets FS Ref" value={assetsHash} />
                )}
              </FactList>
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
