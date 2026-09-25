import { useState } from 'react'
import { LuPlus } from 'react-icons/lu'

import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'

import { AddStorageDialog } from './AddStorageDialog.js'
import { StorageBackendRow } from './StorageBackendRow.js'
import { useStorageBackends } from './useStorageBackends.js'

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
      <div className="mb-2 flex items-center justify-between gap-3">
        <div>
          <h2
            id="storage-backends-title"
            className="text-foreground text-xs font-medium"
          >
            Your buckets
          </h2>
          <p className="text-foreground-alt/60 mt-0.5 text-xs">
            {defaultName
              ? `New Spaces store their data in ${defaultName}.`
              : "New Spaces store their data in the account's own storage."}
          </p>
        </div>
        <DashboardButton
          icon={<LuPlus className="size-3.5" />}
          onClick={() => setAdding(true)}
        >
          Add storage
        </DashboardButton>
      </div>

      <InfoCard>
        {backends.length === 0 ? (
          <p className="text-foreground-alt/60 text-xs">
            Add an S3-compatible bucket, such as Amazon S3, Cloudflare R2, or
            Backblaze B2, to keep Space data in storage you own.
          </p>
        ) : (
          <div className="divide-foreground/8 divide-y">
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
