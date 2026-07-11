import { useCallback, useState } from 'react'
import { LuKey, LuPlus, LuTrash2, LuDownload } from 'react-icons/lu'

import { downloadPemFile } from '@s4wave/web/download.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { cn } from '@s4wave/web/style/utils.js'
import { CredentialProofInput } from '@s4wave/web/ui/credential/CredentialProofInput.js'
import { useCredentialProof } from '@s4wave/web/ui/credential/useCredentialProof.js'
import { truncatePeerId } from '@s4wave/web/ui/credential/auth-utils.js'
import { useMountAccount } from '@s4wave/web/hooks/useMountAccount.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import type { EntityKeypair } from '@s4wave/core/session/session.pb.js'

export interface EntityKeypairsSectionProps {
  // embedded hides the section heading and outer wrapper when rendered inside
  // a parent CollapsibleSection.
  embedded?: boolean
}

// EntityKeypairsSection displays and manages entity keypairs for local sessions.
export function EntityKeypairsSection({
  embedded,
}: EntityKeypairsSectionProps) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)

  const { providerId, accountId } = useSessionInfo(session)
  const accountResource = useMountAccount(providerId, accountId)
  const keypairsResource = useStreamingResource(
    accountResource,
    (account, signal) => account.watchEntityKeypairs({}, signal),
    [],
  )
  const keypairs = (keypairsResource.value?.keypairs ?? [])
    .map((state) => state.keypair)
    .filter((keypair): keypair is EntityKeypair => keypair != null)
  const loading = keypairsResource.loading

  const [showAdd, setShowAdd] = useState(false)
  const [adding, setAdding] = useState(false)
  const [removing, setRemoving] = useState<string | null>(null)
  const [exporting, setExporting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const cred = useCredentialProof()

  const handleAddPassword = useCallback(async () => {
    if (!session || !cred.credential) return
    setAdding(true)
    setError(null)
    try {
      await session.localProvider.addEntityKeypair({
        credential: cred.credential,
      })
      cred.reset()
      setShowAdd(false)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to add keypair')
    }
    setAdding(false)
  }, [session, cred])

  const handleAddBackup = useCallback(async () => {
    const account = accountResource.value
    if (!account) return
    setExporting(true)
    setError(null)
    try {
      const resp = await account.generateBackupKey({})
      if (resp.pemData) {
        const filename = `backup-key-${resp.peerId?.slice(0, 8) ?? 'key'}.pem`
        downloadPemFile(resp.pemData, filename)
      }
      cred.reset()
      setShowAdd(false)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to add backup key')
    }
    setExporting(false)
  }, [accountResource.value, cred])

  const handleRemove = useCallback(
    async (peerId: string) => {
      if (!session || !peerId) return
      setRemoving(peerId)
      setError(null)
      try {
        await session.localProvider.removeEntityKeypair({ peerId })
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : 'Failed to remove keypair')
      }
      setRemoving(null)
    },
    [session],
  )

  const busy = adding || exporting

  const content = (
    <>
      {loading && (
        <p className="text-foreground-alt text-xs">Loading keypairs…</p>
      )}
      {!loading && keypairs.length === 0 && !showAdd && (
        <div className="flex items-center justify-between py-1">
          <p className="text-foreground-alt text-xs">No entity keypairs yet.</p>
          <button
            onClick={() => setShowAdd(true)}
            className="text-brand hover:text-brand/80 text-xs font-medium transition-colors"
          >
            Add keypair
          </button>
        </div>
      )}
      {!loading && keypairs.length > 0 && (
        <div className="space-y-2">
          {keypairs.map((kp) => (
            <KeypairRow
              key={kp.peerId}
              keypair={kp}
              removing={removing === (kp.peerId ?? '')}
              onRemove={handleRemove}
              canRemove={keypairs.length > 1}
            />
          ))}
          {!showAdd && (
            <div className="border-foreground/10 border-t pt-2">
              <button
                onClick={() => setShowAdd(true)}
                className="text-brand hover:text-brand/80 flex items-center gap-1 text-xs font-medium transition-colors"
              >
                <LuPlus className="size-3" />
                Add keypair
              </button>
            </div>
          )}
        </div>
      )}

      {showAdd && (
        <div className="border-foreground/10 space-y-3 border-t pt-3">
          <div className="border-foreground/10 space-y-3 rounded-md border p-3">
            <div>
              <p className="text-foreground text-xs font-medium">
                Password key
              </p>
              <p className="text-foreground-alt mt-1 text-xs">
                Adds a key derived from this password. Use the password to
                recover this local session if it becomes locked.
              </p>
            </div>
            <CredentialProofInput
              password={cred.password}
              onPasswordChange={cred.setPassword}
              showPem={false}
              passwordLabel="Password"
              passwordPlaceholder="Enter password for entity key"
              disabled={busy}
              focusOnMount
            />
            <button
              type="button"
              onClick={() => void handleAddPassword()}
              disabled={busy || !cred.hasCredential}
              className={cn(
                'bg-brand hover:bg-brand/90 rounded px-3 py-1.5 text-xs font-medium text-white transition-colors',
                (busy || !cred.hasCredential) &&
                  'cursor-not-allowed opacity-50',
              )}
            >
              {adding ? 'Adding…' : 'Add password key'}
            </button>
          </div>
          <div className="border-foreground/10 space-y-3 rounded-md border p-3">
            <div>
              <p className="text-foreground text-xs font-medium">Backup key</p>
              <p className="text-foreground-alt mt-1 text-xs">
                Creates and adds a separate backup key, then downloads its PEM
                file. Use the file to recover this local session if the password
                is unavailable. Store it somewhere safe.
              </p>
            </div>
            <button
              type="button"
              onClick={() => void handleAddBackup()}
              disabled={busy}
              className={cn(
                'border-foreground/20 hover:border-brand/30 flex items-center gap-1 rounded border px-3 py-1.5 text-xs font-medium transition-colors',
                busy && 'cursor-not-allowed opacity-50',
              )}
            >
              <LuDownload className="size-3" />
              {exporting ? 'Adding…' : 'Add backup key'}
            </button>
          </div>
          {error && <p className="text-destructive text-xs">{error}</p>}
          <button
            type="button"
            onClick={() => {
              setShowAdd(false)
              setError(null)
              cred.reset()
            }}
            disabled={busy}
            className="text-foreground-alt hover:text-foreground text-xs transition-colors"
          >
            Cancel
          </button>
        </div>
      )}
    </>
  )

  if (embedded) return content

  return (
    <section>
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
          <LuKey className="size-3.5" />
          Entity Keypairs
        </h2>
      </div>
      <InfoCard>{content}</InfoCard>
    </section>
  )
}

interface KeypairRowProps {
  keypair: EntityKeypair
  removing: boolean
  onRemove: (peerId: string) => Promise<void>
  canRemove: boolean
}

function KeypairRow({
  keypair,
  removing,
  onRemove,
  canRemove,
}: KeypairRowProps) {
  const peerId = keypair.peerId ?? ''
  const method = keypair.authMethod === 'pem' ? 'Backup PEM' : 'Password'

  return (
    <div className="flex items-center justify-between gap-2">
      <div className="flex min-w-0 flex-1 items-center gap-2">
        <LuKey className="text-foreground-alt size-3.5 shrink-0" />
        <div className="min-w-0 flex-1">
          <p className="text-foreground truncate text-xs font-medium">
            {truncatePeerId(peerId)}
          </p>
          <p className="text-foreground-alt text-xs">{method}</p>
        </div>
      </div>
      {canRemove && (
        <button
          onClick={() => void onRemove(peerId)}
          disabled={removing}
          className={cn(
            'text-foreground-alt hover:text-destructive flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-xs transition-colors',
            removing && 'cursor-not-allowed opacity-50',
          )}
          title="Remove keypair"
        >
          <LuTrash2 className="size-3" />
        </button>
      )}
    </div>
  )
}
