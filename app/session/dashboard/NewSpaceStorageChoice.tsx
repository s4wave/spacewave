import { useId } from 'react'
import { LuDatabase } from 'react-icons/lu'

import { accountStorageChoice } from '@s4wave/app/session/storage/storage-backend.js'
import { useStorageBackends } from '@s4wave/app/session/storage/useStorageBackends.js'

interface NewSpaceStorageChoiceProps {
  // value is a storage backend id, accountStorageChoice, or empty for the
  // account's default.
  value: string
  onChange: (value: string) => void
}

// NewSpaceStorageChoice picks where the next new Space stores its data. It
// shows only when the account has at least one storage backend.
export function NewSpaceStorageChoice({
  value,
  onChange,
}: NewSpaceStorageChoiceProps) {
  const id = useId()
  const resource = useStorageBackends()
  const backends = resource.value?.storageBackends ?? []
  if (resource.error || backends.length === 0) {
    return null
  }

  const defaultId = resource.value?.defaultStorageBackendId ?? ''
  const defaultName =
    backends.find((b) => b.backend?.id === defaultId)?.backend?.displayName ||
    "the account's own storage"

  return (
    <div className="text-foreground-alt/70 mt-3 flex items-center justify-center gap-2 text-xs">
      <LuDatabase className="size-3.5 shrink-0" aria-hidden="true" />
      <label htmlFor={id} className="select-none">
        New Spaces store data in
      </label>
      <select
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="border-foreground/15 bg-background/40 text-foreground focus:border-brand/50 rounded-md border px-2 py-1 text-xs outline-none"
        data-testid="new-space-storage"
      >
        <option value="">{defaultName} (default)</option>
        {defaultId && (
          <option value={accountStorageChoice}>
            the account's own storage
          </option>
        )}
        {backends
          .filter((b) => b.backend?.id !== defaultId)
          .map((b) => (
            <option key={b.backend?.id} value={b.backend?.id}>
              {b.backend?.displayName}
            </option>
          ))}
      </select>
    </div>
  )
}
