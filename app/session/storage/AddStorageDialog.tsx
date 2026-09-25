import { useCallback, useId, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { StorageCheckResult } from './StorageCheckResult.js'
import {
  buildStorageCorsRule,
  buildStorageLocation,
  describeStorageCheck,
  storagePresets,
  type StorageCheckView,
  type StorageLocationFields,
} from './storage-backend.js'

interface AddStorageDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  // firstBackend preselects using the bucket for new Spaces.
  firstBackend: boolean
}

// AddStorageForm is every input of the add storage form.
interface AddStorageForm extends StorageLocationFields {
  name: string
  accessKeyId: string
  secretAccessKey: string
}

const emptyForm: AddStorageForm = {
  preset: 'aws',
  name: '',
  bucket: '',
  region: '',
  endpoint: '',
  accountId: '',
  prefix: '',
  accessKeyId: '',
  secretAccessKey: '',
}

const inputClass = cn(
  'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm outline-none transition-colors',
  'focus:border-brand/50',
)

// AddStorageDialog adds an S3-compatible bucket as a storage backend. The
// bucket is checked live before it is saved; when the browser cannot reach
// it, the dialog shows the CORS rule the bucket needs.
export function AddStorageDialog({
  open,
  onOpenChange,
  firstBackend,
}: AddStorageDialogProps) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const formId = useId()
  const [form, setForm] = useState<AddStorageForm>(emptyForm)
  const [setDefault, setSetDefault] = useState(firstBackend)
  const [check, setCheck] = useState<StorageCheckView | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  // Any edit makes the last check stale.
  const update = useCallback((patch: Partial<AddStorageForm>) => {
    setForm((prev) => ({ ...prev, ...patch }))
    setCheck(null)
    setError('')
  }, [])

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (busy) return
      if (!next) {
        setForm(emptyForm)
        setCheck(null)
        setError('')
      }
      onOpenChange(next)
    },
    [busy, onOpenChange],
  )

  // Build the request fields, or name the input still missing.
  const buildRequest = useCallback(() => {
    const built = buildStorageLocation(form)
    if (built.missing) {
      setError(`Enter ${built.missing}.`)
      return null
    }
    if (!form.accessKeyId.trim() || !form.secretAccessKey) {
      setError('Enter the access key ID and secret.')
      return null
    }
    return {
      s3: built.location,
      credentials: {
        accessKeyId: form.accessKeyId.trim(),
        secretAccessKey: form.secretAccessKey,
      },
    }
  }, [form])

  const handleCheck = useCallback(async () => {
    const req = buildRequest()
    if (!session || !req) return
    setBusy(true)
    try {
      const resp = await session.checkStorageBackend(req)
      setCheck(describeStorageCheck(resp.result, !isDesktop))
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }, [buildRequest, session])

  const handleAdd = useCallback(async () => {
    const req = buildRequest()
    if (!session || !req) return
    const displayName = form.name.trim() || form.bucket.trim()
    setBusy(true)
    try {
      // The session checks the bucket again and saves only when it passes.
      const resp = await session.addStorageBackend({
        ...req,
        displayName,
        setDefault,
      })
      if (!resp.storageBackendId) {
        setCheck(describeStorageCheck(resp.check, !isDesktop))
        return
      }
      toast.success(`Added ${displayName}`)
      setForm(emptyForm)
      setCheck(null)
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }, [buildRequest, form.bucket, form.name, onOpenChange, session, setDefault])

  const preset =
    storagePresets.find((p) => p.id === form.preset) ?? storagePresets[0]
  const showRegion = form.preset !== 'r2'
  const endpointHint =
    form.preset === 'custom'
      ? 'storage.example.com'
      : 'Optional. Filled from the provider.'

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add storage</DialogTitle>
          <DialogDescription>
            Keep your Spaces&apos; data in an S3-compatible bucket you own. The
            keys follow your account to its other devices.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <Field id={`${formId}-preset`} label="Provider">
            <select
              id={`${formId}-preset`}
              value={form.preset}
              onChange={(e) =>
                update({ preset: e.target.value as AddStorageForm['preset'] })
              }
              className={inputClass}
            >
              {storagePresets.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.label}
                </option>
              ))}
            </select>
          </Field>

          <div className="grid grid-cols-2 gap-3">
            <Field id={`${formId}-bucket`} label="Bucket">
              <input
                id={`${formId}-bucket`}
                value={form.bucket}
                onChange={(e) => update({ bucket: e.target.value })}
                placeholder="my-spaces"
                autoComplete="off"
                className={inputClass}
              />
            </Field>
            {preset.needsAccountId ? (
              <Field id={`${formId}-account`} label="Cloudflare account ID">
                <input
                  id={`${formId}-account`}
                  value={form.accountId}
                  onChange={(e) => update({ accountId: e.target.value })}
                  autoComplete="off"
                  className={inputClass}
                />
              </Field>
            ) : (
              showRegion && (
                <Field id={`${formId}-region`} label="Region">
                  <input
                    id={`${formId}-region`}
                    value={form.region}
                    onChange={(e) => update({ region: e.target.value })}
                    placeholder={preset.regionHint}
                    autoComplete="off"
                    className={inputClass}
                  />
                </Field>
              )
            )}
          </div>

          <Field id={`${formId}-endpoint`} label="Endpoint">
            <input
              id={`${formId}-endpoint`}
              value={form.endpoint}
              onChange={(e) => update({ endpoint: e.target.value })}
              placeholder={endpointHint}
              autoComplete="off"
              className={inputClass}
            />
          </Field>

          <div className="grid grid-cols-2 gap-3">
            <Field id={`${formId}-key`} label="Access key ID">
              <input
                id={`${formId}-key`}
                value={form.accessKeyId}
                onChange={(e) => update({ accessKeyId: e.target.value })}
                autoComplete="off"
                className={inputClass}
              />
            </Field>
            <Field id={`${formId}-secret`} label="Secret access key">
              <input
                id={`${formId}-secret`}
                type="password"
                value={form.secretAccessKey}
                onChange={(e) => update({ secretAccessKey: e.target.value })}
                autoComplete="off"
                className={inputClass}
              />
            </Field>
          </div>

          <details>
            <summary className="text-foreground-alt hover:text-foreground cursor-pointer text-xs">
              Name and key prefix
            </summary>
            <div className="mt-2 grid grid-cols-2 gap-3">
              <Field id={`${formId}-name`} label="Name">
                <input
                  id={`${formId}-name`}
                  value={form.name}
                  onChange={(e) => update({ name: e.target.value })}
                  placeholder={form.bucket || 'The bucket name'}
                  autoComplete="off"
                  className={inputClass}
                />
              </Field>
              <Field id={`${formId}-prefix`} label="Key prefix">
                <input
                  id={`${formId}-prefix`}
                  value={form.prefix}
                  onChange={(e) => update({ prefix: e.target.value })}
                  placeholder="spacewave/"
                  autoComplete="off"
                  className={inputClass}
                />
              </Field>
            </div>
          </details>

          <label className="text-foreground-alt flex items-center gap-2 text-xs select-none">
            <input
              type="checkbox"
              checked={setDefault}
              onChange={(e) => setSetDefault(e.target.checked)}
            />
            Store new Spaces in this bucket
          </label>

          {check && (
            <StorageCheckResult
              check={check}
              corsRule={buildStorageCorsRule(
                form.preset,
                window.location.origin,
              )}
            />
          )}
          {error && <p className="text-destructive text-xs">{error}</p>}
        </div>

        <DialogFooter>
          <button
            type="button"
            onClick={() => void handleCheck()}
            disabled={busy}
            className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors disabled:opacity-50"
          >
            {busy ? 'Checking…' : check ? 'Check again' : 'Check connection'}
          </button>
          <button
            type="button"
            onClick={() => void handleAdd()}
            disabled={busy}
            className={cn(
              'rounded-md border px-4 py-2 text-sm transition-all',
              'border-brand/30 bg-brand/10 hover:bg-brand/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            Add storage
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// Field labels one form input.
function Field({
  id,
  label,
  children,
}: {
  id: string
  label: string
  children: React.ReactNode
}) {
  return (
    <div>
      <label
        className="text-foreground-alt mb-1.5 block text-xs select-none"
        htmlFor={id}
      >
        {label}
      </label>
      {children}
    </div>
  )
}
