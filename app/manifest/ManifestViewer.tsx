import { useMemo } from 'react'
import { LuPackage } from 'react-icons/lu'

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

  return (
    <div className="bg-background-primary flex h-full w-full flex-col overflow-auto">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center border-b px-4">
        <div className="text-foreground flex items-center gap-2 text-sm font-semibold select-none">
          <LuPackage className="h-4 w-4" />
          <span className="tracking-tight">Manifest</span>
        </div>
      </div>
      <div className="flex-1 overflow-auto px-4 py-3">
        <InfoCard>
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
        </InfoCard>
        {manifest?.entrypoint && (
          <InfoCard>
            <div className="space-y-2">
              <CopyableField label="Entrypoint" value={manifest.entrypoint} />
            </div>
          </InfoCard>
        )}
        {(distHash || assetsHash) && (
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
        )}
      </div>
    </div>
  )
}
