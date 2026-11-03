import { useState } from 'react'
import { LuPlus } from '@s4wave/web/images/icons.js'

import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'

import { AddStorageDialog } from './AddStorageDialog.js'
import { StorageBackendRow } from './StorageBackendRow.js'
import { useStorageBackends } from './useStorageBackends.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule3: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// StorageBackendsSection lists the buckets the account stores Spaces in and
// adds new ones. Sessions that cannot hold storage backends see nothing.
export function StorageBackendsSection() {
  const resource = useStorageBackends()
  const [adding, setAdding] = useState(false)

  const value = resource.value
  if (resource.error || !value) {
    return null
  }
  const backends = value.storageBackends ?? []
  const defaultId = value.defaultStorageBackendId ?? ''
  const defaultName =
    backends.find((b) => b.backend?.id === defaultId)?.backend?.displayName ??
    ''

  return (
    <section
      aria-labelledby="storage-backends-title"
      data-testid="storage-backends"
    >
      <div sx={stylexStyles2.rule1}>
        <div>
          <h2 id="storage-backends-title" sx={stylexStyles2.rule2}>
            Your buckets
          </h2>
          <p sx={stylexStyles2.rule3}>
            {defaultName
              ? `New Spaces store their data in ${defaultName}.`
              : "New Spaces store their data in the account's own storage."}
          </p>
        </div>
        <DashboardButton
          icon={<LuPlus sx={stylexStyles.rule1} />}
          onClick={() => setAdding(true)}
        >
          Add storage
        </DashboardButton>
      </div>

      <InfoCard>
        {backends.length === 0 ? (
          <p sx={stylexStyles2.rule4}>
            Add an S3-compatible bucket, such as Amazon S3, Cloudflare R2, or
            Backblaze B2, to keep Space data in storage you own.
          </p>
        ) : (
          <div
            className={
              /* TODO(stylex): a descendant, group, peer or compound selector: divide-y */ 'divide-foreground/8 divide-y'
            }
          >
            {backends.map((info) => (
              <StorageBackendRow
                key={info.backend?.id}
                info={info}
                isDefault={info.backend?.id === defaultId}
              />
            ))}
          </div>
        )}
      </InfoCard>

      <AddStorageDialog
        open={adding}
        onOpenChange={setAdding}
        firstBackend={backends.length === 0}
      />
    </section>
  )
}
