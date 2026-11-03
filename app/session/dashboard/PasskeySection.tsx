import { useCallback, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import {
  LuFingerprint,
  LuShieldCheck,
  LuShieldAlert,
} from '@s4wave/web/images/icons.js'
import { startRegistration } from '@simplewebauthn/browser'

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

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule4: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule5: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-green-500']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-green-500']} 5%, transparent)`,
  },
  rule6: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-yellow-500']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-yellow-500']} 5%, transparent)`,
  },
  rule7: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    color: themeColors['--color-green-500'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule3: {
    color: themeColors['--color-green-500'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
    color: themeColors['--color-yellow-500'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: { justifyContent: 'center', width: '100%' },
})

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
          handoff.prfCapable && handoff.prfOutput
            ? base64ToBytes(handoff.prfOutput)
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
          const prfWrapped = prfOutput
            ? await wrapPemWithPasskeyPrf(spacewave, entity.pem, prfOutput)
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

        <div>
          {status === 'idle' && (
            <DashboardButton
              icon={<LuFingerprint sx={stylexStyles2.rule1} />}
              onClick={() => void handleRegister()}
              sx={[nativeChildSpacingNext.s3, stylexStyles.rule1]}
            >
              Start registration
            </DashboardButton>
          )}

          {status === 'loading' && (
            <div sx={[nativeChildSpacingNext.s3, stylexStyles3.rule1]}>
              <LoadingInline
                label={
                  isDesktop
                    ? 'Complete the passkey step in your browser'
                    : 'Waiting for authenticator'
                }
                tone="muted"
                size="sm"
              />
            </div>
          )}

          {status === 'success' && (
            <div sx={nativeChildSpacingNext.s3}>
              <div sx={[nativeChildSpacing.s2, stylexStyles3.rule2]}>
                <LuFingerprint sx={stylexStyles2.rule2} />
                <span sx={stylexStyles3.rule3}>Passkey registered</span>
              </div>
              {prfResult !== null && (
                <div
                  sx={[
                    nativeChildSpacing.s2,
                    [
                      stylexStyles3.rule4,
                      prfResult ? stylexStyles3.rule5 : stylexStyles3.rule6,
                    ],
                  ]}
                >
                  {prfResult ? (
                    <LuShieldCheck sx={stylexStyles2.rule3} />
                  ) : (
                    <LuShieldAlert sx={stylexStyles2.rule4} />
                  )}
                  <span sx={stylexStyles3.rule7}>
                    {prfResult
                      ? 'Authenticator PRF is active: your PEM was encrypted locally before upload'
                      : 'Server-assisted: this passkey is stored with server PEM wrapping'}
                  </span>
                </div>
              )}
              <DashboardButton
                icon={<></>}
                onClick={() => handleClose(false)}
                sx={[nativeChildSpacing.s2, stylexStyles.rule1]}
              >
                Done
              </DashboardButton>
            </div>
          )}

          {status === 'error' && (
            <div sx={nativeChildSpacingNext.s3}>
              <p sx={[nativeChildSpacing.s2, stylexStyles3.rule8]}>{error}</p>
              <DashboardButton
                icon={<LuFingerprint sx={stylexStyles2.rule1} />}
                onClick={() => void handleRegister()}
                sx={[nativeChildSpacing.s2, stylexStyles.rule1]}
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})
