import { useCallback, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import { LuFingerprint, LuShieldCheck, LuShieldAlert } from 'react-icons/lu'
import { startRegistration } from '@simplewebauthn/browser'

import { cn } from '@s4wave/web/style/utils.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import {
  base64ToBytes,
  generateAuthKeypairs,
} from '@s4wave/app/provider/spacewave/keypair-utils.js'
import { withSpacewaveProvider } from '@s4wave/app/provider/spacewave/auth-flow-shared.js'
import {
  addRegistrationPrfInput,
  generatePasskeyPrfSalt,
  getCredentialPrfOutput,
  wrapPemWithPasskeyPrf,
} from '@s4wave/app/provider/spacewave/passkey-prf.js'

export interface PasskeySectionProps {
  account: Resource<Account>
  open: boolean
  onOpenChange: (open: boolean) => void
}

// PasskeySection handles passkey registration via WebAuthn.
export function PasskeySection({
  account,
  open,
  onOpenChange,
}: PasskeySectionProps) {
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const [status, setStatus] = useState<
    'idle' | 'loading' | 'success' | 'error'
  >('idle')
  const [error, setError] = useState<string | null>(null)
  const [prfResult, setPrfResult] = useState<boolean | null>(null)

  const handleRegister = useCallback(async () => {
    const acc = account.value
    if (!acc || !root) return

    setStatus('loading')
    setError(null)
    setPrfResult(null)

    try {
      let credentialJson: string
      let prfSalt: string
      let prfOutput: Uint8Array | null

      if (isDesktop) {
        const handoff = await acc.startDesktopPasskeyRegisterHandoff()
        credentialJson = handoff.credentialJson ?? ''
        if (!credentialJson) {
          throw new Error('Desktop passkey did not return a credential')
        }
        prfSalt = handoff.prfSalt ?? ''
        prfOutput =
          handoff.prfCapable && handoff.prfOutput ?
            base64ToBytes(handoff.prfOutput)
          : null
      } else {
        const optionsResp = await acc.passkeyRegisterOptions()
        const optionsJSON = optionsResp.optionsJson ?? ''
        if (!optionsJSON) {
          throw new Error('Empty options from server')
        }
        const parsedOptions: unknown = JSON.parse(optionsJSON)
        prfSalt = await withSpacewaveProvider(root, (spacewave) =>
          generatePasskeyPrfSalt(spacewave),
        )
        const options = addRegistrationPrfInput(
          parsedOptions as Record<string, unknown>,
          prfSalt,
        ) as unknown as Parameters<typeof startRegistration>[0]['optionsJSON']
        const credential = await startRegistration({ optionsJSON: options })
        credentialJson = JSON.stringify(credential)
        prfOutput = getCredentialPrfOutput(credential.clientExtensionResults)
      }

      const { entity, prfWrapped } = await withSpacewaveProvider(
        root,
        async (spacewave) => {
          const { entity } = await generateAuthKeypairs(spacewave)
          const prfWrapped =
            prfOutput ?
              await wrapPemWithPasskeyPrf(spacewave, entity.pem, prfOutput)
            : null
          return { entity, prfWrapped }
        },
      )
      setPrfResult(!!prfWrapped)

      const verifyResp = await acc.passkeyRegisterVerify({
        credentialJson,
        prfCapable: !!prfWrapped,
        encryptedPrivkey:
          prfWrapped?.encryptedPrivkey ?? entity.custodiedPemBase64,
        peerId: entity.peerId,
        authParams: prfWrapped?.authParams ?? '',
        prfSalt: prfWrapped ? prfSalt : '',
      })

      if (verifyResp.credentialId) {
        setStatus('success')
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      if (
        msg.includes('NotAllowedError') ||
        msg.includes('cancelled') ||
        msg.includes('cancel') ||
        msg.includes('abort')
      ) {
        setStatus('idle')
        return
      }
      setError(msg)
      setStatus('error')
    }
  }, [account.value, root])

  const handleClose = useCallback(
    (open: boolean) => {
      if (!open) {
        setStatus('idle')
        setError(null)
        setPrfResult(null)
      }
      onOpenChange(open)
    },
    [onOpenChange],
  )

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Register passkey</DialogTitle>
          <DialogDescription>
            Use your device biometrics or a hardware security key to add a
            passkey to your account.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          {status === 'idle' && (
            <DashboardButton
              icon={<LuFingerprint className="h-4 w-4" />}
              onClick={() => void handleRegister()}
              className="w-full justify-center"
            >
              Start registration
            </DashboardButton>
          )}

          {status === 'loading' && (
            <div className="flex items-center justify-center py-4">
              <LoadingInline
                label={
                  isDesktop ?
                    'Complete the passkey step in your browser'
                  : 'Waiting for authenticator'
                }
                tone="muted"
                size="sm"
              />
            </div>
          )}

          {status === 'success' && (
            <div className="space-y-2">
              <div className="flex items-center gap-2 py-2">
                <LuFingerprint className="h-5 w-5 text-green-500" />
                <span className="text-foreground text-sm font-medium">
                  Passkey registered
                </span>
              </div>
              {prfResult !== null && (
                <div
                  className={cn(
                    'flex items-center gap-2 rounded-md border px-3 py-2',
                    prfResult ?
                      'border-green-500/20 bg-green-500/5'
                    : 'border-yellow-500/20 bg-yellow-500/5',
                  )}
                >
                  {prfResult ?
                    <LuShieldCheck className="h-4 w-4 shrink-0 text-green-500" />
                  : <LuShieldAlert className="h-4 w-4 shrink-0 text-yellow-500" />
                  }
                  <span className="text-foreground-alt text-xs">
                    {prfResult ?
                      'Authenticator PRF is active: your PEM was encrypted locally before upload'
                    : 'Server-assisted: this passkey is stored with server PEM wrapping'
                    }
                  </span>
                </div>
              )}
              <DashboardButton
                icon={<></>}
                onClick={() => handleClose(false)}
                className="w-full justify-center"
              >
                Done
              </DashboardButton>
            </div>
          )}

          {status === 'error' && (
            <div className="space-y-2">
              <p className="text-destructive text-xs">{error}</p>
              <DashboardButton
                icon={<LuFingerprint className="h-4 w-4" />}
                onClick={() => void handleRegister()}
                className="w-full justify-center"
              >
                Try again
              </DashboardButton>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
