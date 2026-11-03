import { useCallback, useId, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

import { SessionContext } from '@s4wave/web/contexts/contexts.js'

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

import * as stylex from '@stylexjs/stylex'
import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule2: {
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    cursor: 'pointer',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    paddingBlock: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 3.5)`,
    },
  },
  rule4: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule5: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule7: {
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule8: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule9: {
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
})

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

        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
          }
        >
          <Field id={`${formId}-preset`} label="Provider">
            <select
              id={`${formId}-preset`}
              value={form.preset}
              onChange={(e) =>
                update({ preset: e.target.value as AddStorageForm['preset'] })
              }
              sx={[stylexStyles.rule1]}
            >
              {storagePresets.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.label}
                </option>
              ))}
            </select>
          </Field>

          <div sx={stylexStyles.rule2}>
            <Field id={`${formId}-bucket`} label="Bucket">
              <input
                id={`${formId}-bucket`}
                value={form.bucket}
                onChange={(e) => update({ bucket: e.target.value })}
                placeholder="my-spaces"
                autoComplete="off"
                sx={[stylexStyles.rule1]}
              />
            </Field>
            {preset.needsAccountId ? (
              <Field id={`${formId}-account`} label="Cloudflare account ID">
                <input
                  id={`${formId}-account`}
                  value={form.accountId}
                  onChange={(e) => update({ accountId: e.target.value })}
                  autoComplete="off"
                  sx={[stylexStyles.rule1]}
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
                    sx={[stylexStyles.rule1]}
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
              sx={[stylexStyles.rule1]}
            />
          </Field>

          <div sx={stylexStyles.rule2}>
            <Field id={`${formId}-key`} label="Access key ID">
              <input
                id={`${formId}-key`}
                value={form.accessKeyId}
                onChange={(e) => update({ accessKeyId: e.target.value })}
                autoComplete="off"
                sx={[stylexStyles.rule1]}
              />
            </Field>
            <Field id={`${formId}-secret`} label="Secret access key">
              <input
                id={`${formId}-secret`}
                type="password"
                value={form.secretAccessKey}
                onChange={(e) => update({ secretAccessKey: e.target.value })}
                autoComplete="off"
                sx={[stylexStyles.rule1]}
              />
            </Field>
          </div>

          <details>
            <summary sx={stylexStyles.rule3}>Name and key prefix</summary>
            <div sx={stylexStyles.rule4}>
              <Field id={`${formId}-name`} label="Name">
                <input
                  id={`${formId}-name`}
                  value={form.name}
                  onChange={(e) => update({ name: e.target.value })}
                  placeholder={form.bucket || 'The bucket name'}
                  autoComplete="off"
                  sx={[stylexStyles.rule1]}
                />
              </Field>
              <Field id={`${formId}-prefix`} label="Key prefix">
                <input
                  id={`${formId}-prefix`}
                  value={form.prefix}
                  onChange={(e) => update({ prefix: e.target.value })}
                  placeholder="spacewave/"
                  autoComplete="off"
                  sx={[stylexStyles.rule1]}
                />
              </Field>
            </div>
          </details>

          <label sx={stylexStyles.rule5}>
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
          {error && <p sx={stylexStyles.rule6}>{error}</p>}
        </div>

        <DialogFooter>
          <button
            type="button"
            onClick={() => void handleCheck()}
            disabled={busy}
            sx={stylexStyles.rule7}
          >
            {busy ? 'Checking…' : check ? 'Check again' : 'Check connection'}
          </button>
          <button
            type="button"
            onClick={() => void handleAdd()}
            disabled={busy}
            sx={[stylexStyles.rule8]}
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
  sx,
  id,
  label,
  children,
}: {
  id: string
  label: string
  children: React.ReactNode
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={sx}>
      <label sx={stylexStyles.rule9} htmlFor={id}>
        {label}
      </label>
      {children}
    </div>
  )
}
