import { LuCloud } from '@s4wave/web/images/icons.js'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { Root } from '@s4wave/sdk/root/root.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { Button } from '@s4wave/web/ui/button.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import {
  mountCdnImageSpace,
  type CdnImageSpace,
} from '../vm/cdn-image-space.js'
import {
  formatV86ImageLabel,
  getV86CatalogErrorCopy,
  listCdnV86Images,
  type V86ImageEntry,
} from '../vm/v86-images.js'
import { V86ImageRow } from './V86ImageRow.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import { colors as themeColors } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    color: themeColors['--color-destructive'],
  },
  rule2: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule3: {
    '--tw-leading': 'var(--leading-relaxed)',
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: themeLayout['--spacing'],
  },
  rule4: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
    maxHeight: themeLayout['--max-height-wizard'],
    overflowY: 'auto',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: { marginTop: `calc(${themeLayout['--spacing']} * 3)` },
})

// V86CatalogDialogProps describes the catalog a new VM picks an image from.
export interface V86CatalogDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  cdnId: string
  // selectedKey is the catalog object key the wizard has chosen, if any.
  selectedKey: string
  onSelect: (objectKey: string) => void
}

// V86CatalogDialog lists the VM images published in a CDN catalog.
export function V86CatalogDialog({
  open,
  onOpenChange,
  cdnId,
  selectedKey,
  onSelect,
}: V86CatalogDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>VM image catalog</DialogTitle>
          <DialogDescription>
            Published images download as the VM reads them.
          </DialogDescription>
        </DialogHeader>
        <CatalogEntries
          cdnId={cdnId}
          selectedKey={selectedKey}
          onSelect={onSelect}
        />
      </DialogContent>
    </Dialog>
  )
}

// CatalogEntries mounts the catalog Space and lists its images, following
// each change to the catalog.
function CatalogEntries({
  cdnId,
  selectedKey,
  onSelect,
}: Pick<V86CatalogDialogProps, 'cdnId' | 'selectedKey' | 'onSelect'>) {
  const catalogResource = useResource(
    useRootResource(),
    async (root: Root, signal: AbortSignal, cleanup) =>
      cleanup(await mountCdnImageSpace(root, cdnId, signal)),
    [cdnId],
  )
  const entriesResource = useStreamingResource(
    catalogResource,
    async function* (
      catalog: CdnImageSpace,
      signal: AbortSignal,
    ): AsyncGenerator<V86ImageEntry[]> {
      yield await listCdnV86Images(catalog.space, signal)
      for await (const _state of catalog.space.watchSpaceState({}, signal)) {
        yield await listCdnV86Images(catalog.space, signal)
      }
    },
    [],
  )
  const entries = entriesResource.value
  const loadError = catalogResource.error ?? entriesResource.error

  if (loadError) {
    const copy = getV86CatalogErrorCopy(loadError)
    return (
      <div role="alert" sx={stylexStyles3.rule1}>
        <div sx={stylexStyles3.rule2}>{copy.title}</div>
        <p sx={stylexStyles3.rule3}>{copy.detail}</p>
        {!copy.unpublished && (
          <Button
            variant="destructiveOutline"
            size="toolbarWide"
            onClick={() => {
              if (catalogResource.error) catalogResource.retry()
              if (entriesResource.error) entriesResource.retry()
            }}
            sx={stylexStyles.rule1}
          >
            Retry
          </Button>
        )}
      </div>
    )
  }
  if (!entries) {
    return (
      <LoadingCard
        view={{
          state: 'active',
          title: 'Loading image catalog',
          detail: 'Looking for published VM images.',
        }}
      />
    )
  }
  if (entries.length === 0) {
    return (
      <InfoCard
        icon={<LuCloud sx={stylexStyles2.rule1} />}
        title="No VM images are published yet"
      >
        <p sx={stylexStyles3.rule4}>
          This image catalog has no published images yet.
        </p>
      </InfoCard>
    )
  }
  return (
    <div sx={stylexStyles3.rule5}>
      {entries.map((entry) => (
        <V86ImageRow
          key={entry.objectKey}
          icon={<LuCloud />}
          title={formatV86ImageLabel(entry.image)}
          detail={describeEntry(entry)}
          selected={entry.objectKey === selectedKey}
          onSelect={() => onSelect(entry.objectKey)}
        />
      ))}
    </div>
  )
}

// describeEntry summarizes a catalog image's distribution and tags.
function describeEntry(entry: V86ImageEntry): string {
  if (entry.metadataError) return 'Image details are unavailable.'
  return [entry.image.distro, entry.image.tags?.join(', ')]
    .filter(Boolean)
    .join(' · ')
}
