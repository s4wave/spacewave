import { useCallback, useEffect, useRef, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import { FcGoogle } from '@s4wave/web/images/icons.js'
import {
  LuArrowUpRight,
  LuCircleAlert,
  LuGithub,
} from '@s4wave/web/images/icons.js'

import {
  useResourceValue,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import { AccountEscalationIntentKind } from '@s4wave/sdk/account/account.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { useCloudProviderConfig } from '@s4wave/app/provider/spacewave/useSpacewaveAuth.js'
import {
  AuthConfirmDialog,
  buildEntityCredential,
  type AuthCredential,
} from './AuthConfirmDialog.js'
import { startSSOPopupFlow, type SSOPopupFlow } from './sso-popup.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 60%, transparent)`,
  },
  rule4: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule6: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule7: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule8: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule9: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule10: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule11: {
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
  },
  rule12: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
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
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-destructive'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

type SSOProvider = 'google' | 'github'

interface SSOLinkDialogProps {
  open: boolean
  provider: SSOProvider
  account: Resource<Account>
  retainStepUp?: boolean
  onOpenChange: (open: boolean) => void
}

function getProviderLabel(provider: SSOProvider): string {
  return provider === 'google' ? 'Google' : 'GitHub'
}

function ProviderIcon(props: {
  provider: SSOProvider
  sx?: import('@stylexjs/stylex').StyleXStyles
}) {
  if (props.provider === 'google') {
    return <FcGoogle sx={props.sx} />
  }
  return <LuGithub sx={props.sx} />
}

export function SSOLinkDialog({
  open,
  provider,
  account,
  retainStepUp = false,
  onOpenChange,
}: SSOLinkDialogProps) {
  const cloudProviderConfig = useCloudProviderConfig()
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const flowRef = useRef<SSOPopupFlow | null>(null)
  const desktopAbortRef = useRef<AbortController | null>(null)
  const [code, setCode] = useState('')
  const [waiting, setWaiting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const cleanupFlow = useCallback(() => {
    flowRef.current?.cancel()
    flowRef.current = null
    desktopAbortRef.current?.abort()
    desktopAbortRef.current = null
  }, [])

  useEffect(() => {
    return cleanupFlow
  }, [cleanupFlow])

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) {
        cleanupFlow()
        setCode('')
        setWaiting(false)
        setError(null)
      }
      onOpenChange(next)
    },
    [cleanupFlow, onOpenChange],
  )

  const handleConfirm = useCallback(
    async (credential: AuthCredential) => {
      const acc = account.value
      const accountBaseUrl = cloudProviderConfig?.accountBaseUrl ?? ''
      if (!acc || !code || !accountBaseUrl) {
        throw new Error('SSO linking is not configured')
      }
      const req = {
        provider,
        code,
        redirectUri: `${accountBaseUrl}/auth/sso/callback`,
      } as Parameters<Account['linkSSO']>[0]
      const entityCredential = buildEntityCredential(credential)
      if (entityCredential) {
        req.credential = entityCredential
      }
      await acc.linkSSO(req)
      toast.success(`Linked ${getProviderLabel(provider)}`)
    },
    [account.value, cloudProviderConfig?.accountBaseUrl, code, provider],
  )

  const handleStart = useCallback(() => {
    cleanupFlow()

    if (isDesktop) {
      if (!session) {
        setError('Session is not ready')
        return
      }
      const controller = new AbortController()
      desktopAbortRef.current = controller
      setError(null)
      setWaiting(true)
      void session.spacewave
        .startDesktopSSOLink({ ssoProvider: provider }, controller.signal)
        .then((resp) => {
          if (controller.signal.aborted) {
            return
          }
          desktopAbortRef.current = null
          const nextCode = resp.code ?? ''
          setWaiting(false)
          if (!nextCode) {
            setError('Desktop SSO link did not return an authorization code')
            return
          }
          setError(null)
          setCode(nextCode)
        })
        .catch((err) => {
          if (controller.signal.aborted) {
            return
          }
          desktopAbortRef.current = null
          setWaiting(false)
          setError(err instanceof Error ? err.message : 'SSO flow failed')
        })
      return
    }

    const ssoBaseUrl = cloudProviderConfig?.ssoBaseUrl ?? ''
    if (!ssoBaseUrl) {
      setError('SSO is not configured')
      return
    }

    try {
      const flow = startSSOPopupFlow({
        provider,
        ssoBaseUrl,
        origin: window.location.origin,
        mode: 'link',
      })
      flowRef.current = flow
      setError(null)
      setWaiting(true)
      void flow.waitForResult
        .then((nextCode) => {
          setWaiting(false)
          setError(null)
          setCode(nextCode)
        })
        .catch((err) => {
          setWaiting(false)
          setError(err instanceof Error ? err.message : 'SSO flow failed')
        })
    } catch (err) {
      setWaiting(false)
      setError(err instanceof Error ? err.message : 'SSO flow failed')
    }
  }, [cleanupFlow, cloudProviderConfig?.ssoBaseUrl, provider, session])

  if (code) {
    return (
      <AuthConfirmDialog
        open={open}
        onOpenChange={handleOpenChange}
        title={`Link ${getProviderLabel(provider)}`}
        description={`Confirm your identity to link ${getProviderLabel(provider)} to this account.`}
        confirmLabel={`Link ${getProviderLabel(provider)}`}
        intent={{
          kind: AccountEscalationIntentKind.AccountEscalationIntentKind_ACCOUNT_ESCALATION_INTENT_KIND_LINK_SSO,
          title: `Link ${getProviderLabel(provider)}`,
          description: `Confirm your identity to link ${getProviderLabel(provider)} to this account.`,
          provider,
        }}
        onConfirm={handleConfirm}
        account={account}
        retainAfterClose={retainStepUp}
      />
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle variant="withIcon">
            <ProviderIcon provider={provider} sx={stylexStyles2.rule1} />
            Link {getProviderLabel(provider)}
          </DialogTitle>
          <DialogDescription>
            {isDesktop
              ? `Sign in with ${getProviderLabel(provider)} in your system
               browser, then confirm the link in this window.`
              : `Open the provider sign-in page in a popup, then confirm the
               link in this window.`}
          </DialogDescription>
        </DialogHeader>

        <div>
          <div sx={[nativeChildSpacing.s3, stylexStyles3.rule1]}>
            <div sx={stylexStyles3.rule2}>
              <div sx={stylexStyles3.rule3}>
                <ProviderIcon provider={provider} sx={stylexStyles2.rule1} />
              </div>
              <div>
                <p sx={[nativeChildSpacing.s1, stylexStyles3.rule4]}>
                  Continue with {getProviderLabel(provider)}
                </p>
                <p sx={[nativeChildSpacing.s1, stylexStyles3.rule5]}>
                  {isDesktop
                    ? `The provider page opens in your system browser so this
                     app keeps its current account state and unlocked-key
                     context.`
                    : `The provider page opens in a separate window so this
                     session keeps its current account state and unlocked-key
                     context.`}
                </p>
              </div>
            </div>
          </div>

          {waiting && (
            <div sx={[nativeChildSpacing.s3, stylexStyles3.rule6]}>
              <div sx={stylexStyles3.rule7}>
                <Spinner variant="brand" />
                <p sx={stylexStyles3.rule4}>
                  Waiting for {getProviderLabel(provider)}...
                </p>
              </div>
              <p sx={stylexStyles3.rule8}>
                {isDesktop
                  ? `Finish the provider sign-in in your browser, then return
                   here to confirm the account link.`
                  : `Finish the provider sign-in in the popup, then return
                   here to confirm the account link.`}
              </p>
            </div>
          )}

          {error && (
            <div sx={[nativeChildSpacing.s3, stylexStyles3.rule9]}>
              <div sx={stylexStyles3.rule7}>
                <LuCircleAlert sx={stylexStyles.rule1} />
                <p sx={stylexStyles3.rule10}>{error}</p>
              </div>
            </div>
          )}
        </div>

        <DialogFooter>
          <button
            type="button"
            onClick={() => onOpenChange(false)}
            sx={stylexStyles3.rule11}
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleStart}
            sx={[stylexStyles3.rule12]}
          >
            <LuArrowUpRight sx={stylexStyles.rule2} />
            {waiting
              ? 'Open again'
              : isDesktop
                ? 'Continue in browser'
                : 'Continue in popup'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
