import { useMemo } from 'react'
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

// ManifestViewer displays a bldr Manifest world object.
export function ManifestViewer({
  objectInfo: _objectInfo,
  objectState,
}: ObjectViewerComponentProps) {
  const manifest = useForgeBlockData(objectState, Manifest)
  const meta = manifest?.meta

  const distHash = useMemo(
    () => formatBlockRefHash(manifest?.distFsRef),
    [manifest?.distFsRef],
  )
  const assetsHash = useMemo(
    () => formatBlockRefHash(manifest?.assetsFsRef),
    [manifest?.assetsFsRef],
  )

  const headerMeta = [
    meta?.platformId,
    meta?.rev !== undefined ? `rev ${meta.rev}` : null,
  ]
    .filter(Boolean)
    .join(' · ')

  const hasMeta = !!(
    meta?.manifestId ||
    meta?.buildType ||
    meta?.platformId ||
    meta?.rev !== undefined ||
    meta?.description
  )
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
            <section>
              <div className="mb-2 flex items-center justify-between">
                <h2 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
                  <LuTerminal className="size-3.5" />
                  Entrypoint
                </h2>
              </div>
              <InfoCard>
                <CopyableField label="Path" value={manifest.entrypoint} />
              </InfoCard>
            </section>
          )}
          {hasStorage && (
            <section>
              <div className="mb-2 flex items-center justify-between">
                <h2 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
                  <LuFolderTree className="size-3.5" />
                  Storage
                </h2>
              </div>
              <InfoCard>
                <div className="space-y-2">
                  {distHash && (
                    <CopyableField label="Dist FS Ref" value={distHash} />
                  )}
                  {assetsHash && (
                    <CopyableField label="Assets FS Ref" value={assetsHash} />
                  )}
                </div>
              </InfoCard>
            </section>
          )}
          {hasMeta && (
            <section>
              <div className="mb-2 flex items-center justify-between">
                <h2 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
                  <LuTag className="size-3.5" />
                  Metadata
                </h2>
              </div>
              <InfoCard>
                <div className="space-y-2">
                  {meta?.manifestId && (
                    <CopyableField
                      label="Manifest ID"
                      value={meta.manifestId}
                    />
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
                    <CopyableField
                      label="Description"
                      value={meta.description}
                    />
                  )}
                </div>
              </InfoCard>
            </section>
          )}
        </div>
      </div>
    </div>
  )
}
