import { useCallback, useEffect, useRef, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import { FcGoogle } from 'react-icons/fc'
import { LuArrowUpRight, LuCircleAlert, LuGithub } from 'react-icons/lu'

import {
  useResourceValue,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import { AccountEscalationIntentKind } from '@s4wave/sdk/account/account.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { cn } from '@s4wave/web/style/utils.js'
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

function ProviderIcon(props: { provider: SSOProvider; className?: string }) {
  if (props.provider === 'google') {
    return <FcGoogle className={props.className} />
  }
  return <LuGithub className={props.className} />
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
          <DialogTitle className="flex items-center gap-2">
            <ProviderIcon provider={provider} className="h-5 w-5" />
            Link {getProviderLabel(provider)}
          </DialogTitle>
          <DialogDescription>
            {isDesktop ?
              `Sign in with ${getProviderLabel(provider)} in your system
               browser, then confirm the link in this window.`
            : `Open the provider sign-in page in a popup, then confirm the
               link in this window.`
            }
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div className="border-foreground/10 bg-background/30 rounded-lg border p-4">
            <div className="flex items-start gap-3">
              <div className="border-foreground/10 bg-background/60 flex h-10 w-10 shrink-0 items-center justify-center rounded-full border">
                <ProviderIcon provider={provider} className="h-5 w-5" />
              </div>
              <div className="space-y-1">
                <p className="text-foreground text-sm font-medium">
                  Continue with {getProviderLabel(provider)}
                </p>
                <p className="text-foreground-alt text-xs">
                  {isDesktop ?
                    `The provider page opens in your system browser so this
                     app keeps its current account state and unlocked-key
                     context.`
                  : `The provider page opens in a separate window so this
                     session keeps its current account state and unlocked-key
                     context.`
                  }
                </p>
              </div>
            </div>
          </div>

          {waiting && (
            <div className="border-brand/20 bg-brand/5 rounded-lg border px-3 py-2.5">
              <div className="flex items-center gap-2">
                <Spinner className="text-brand" />
                <p className="text-foreground text-sm font-medium">
                  Waiting for {getProviderLabel(provider)}...
                </p>
              </div>
              <p className="text-foreground-alt mt-1 text-xs">
                {isDesktop ?
                  `Finish the provider sign-in in your browser, then return
                   here to confirm the account link.`
                : `Finish the provider sign-in in the popup, then return
                   here to confirm the account link.`
                }
              </p>
            </div>
          )}

          {error && (
            <div className="border-destructive/20 bg-destructive/5 rounded-lg border px-3 py-2.5">
              <div className="flex items-center gap-2">
                <LuCircleAlert className="text-destructive h-4 w-4" />
                <p className="text-destructive text-sm">{error}</p>
              </div>
            </div>
          )}
        </div>

        <DialogFooter>
          <button
            type="button"
            onClick={() => onOpenChange(false)}
            className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleStart}
            className={cn(
              'rounded-md border px-4 py-2 text-sm transition-all',
              'border-brand/30 bg-brand/10 hover:bg-brand/20',
              'inline-flex items-center gap-2',
            )}
          >
            <LuArrowUpRight className="h-4 w-4" />
            {waiting ?
              'Open again'
            : isDesktop ?
              'Continue in browser'
            : 'Continue in popup'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
