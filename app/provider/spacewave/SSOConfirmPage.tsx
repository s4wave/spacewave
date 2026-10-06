/* eslint-disable react-doctor/no-giant-component */
import { useCallback, useMemo, useState, type ReactNode } from 'react'
import { isDesktop } from '@aptre/bldr'
import { LuArrowLeft, LuCheck, LuUserPlus } from 'react-icons/lu'

import { useNavigate, useParams } from '@s4wave/web/router/router.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import type { Root } from '@s4wave/sdk/root/root.js'
import type { HandoffRequest } from '@s4wave/core/session/handoff/handoff.pb.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import { HandoffComplete } from '@s4wave/app/auth/HandoffComplete.js'
import {
  completeStoredHandoff,
  getAuthReturnPath,
} from '@s4wave/app/auth/handoff-state.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { AuthProgressCard } from '@s4wave/web/ui/credential/AuthProgressCard.js'
import {
  getPendingSSOState,
  clearPendingSSOState,
  type PendingSSOState,
} from './sso-state.js'
import { generateAuthKeypairs, wrapPemWithPin } from './keypair-utils.js'
import { OptionalPinLock } from './OptionalPinLock.js'
import {
  AuthCard,
  AuthPrimaryActionButton,
  AuthSecondaryActionButton,
  AuthStatusPanel,
  authInputClassName,
  getProviderLabel,
  getErrorMessage,
  isUsernameTakenError,
  loginWithEntityPem,
  normalizeUsernameInput,
  ProviderIcon,
  validateOptionalPin,
  validateUsername,
  withSpacewaveProvider,
} from './auth-flow-shared.js'

type SSOConfirmState =
  | { step: 'form' }
  | { step: 'confirm'; username: string }
  | { step: 'creating' }
  | { step: 'logging_in' }
  | { step: 'handoff_complete'; request: HandoffRequest; sessionIndex: number }
  | { step: 'error'; message: string }

const SIGNUP_HIGHLIGHTS = ['End-to-end encrypted', 'Local-first', 'Open source']

type SignupField = 'username' | 'pin'

interface SignupFieldError {
  field: SignupField
  error: string
}

/** validateSignup checks the username and optional PIN, returning the first failure. */
function validateSignup(
  username: string,
  pin: string,
  confirmPin: string,
): SignupFieldError | null {
  const usernameError = validateUsername(username)
  if (usernameError) return { field: 'username', error: usernameError }
  const pinError = validateOptionalPin(pin, confirmPin)
  if (pinError) return { field: 'pin', error: pinError }
  return null
}

/** useSignupFields holds the username and PIN inputs with their errors. */
function useSignupFields() {
  const [username, setUsername] = useState('')
  const [usernameError, setUsernameError] = useState('')
  const [pin, setPin] = useState('')
  const [confirmPin, setConfirmPin] = useState('')
  const [pinError, setPinError] = useState('')

  const handleUsernameChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const next = normalizeUsernameInput(e.target.value)
      setUsername(next.username)
      setUsernameError(next.error)
    },
    [],
  )

  const handlePinChange = useCallback((value: string) => {
    setPin(value)
    setPinError('')
  }, [])

  const handleConfirmPinChange = useCallback((value: string) => {
    setConfirmPin(value)
    setPinError('')
  }, [])

  const reportError = useCallback(({ field, error }: SignupFieldError) => {
    if (field === 'username') {
      setUsernameError(error)
    } else {
      setPinError(error)
    }
  }, [])

  return {
    username,
    usernameError,
    pin,
    confirmPin,
    pinError,
    handleUsernameChange,
    handlePinChange,
    handleConfirmPinChange,
    reportError,
  }
}

type SignupFields = ReturnType<typeof useSignupFields>

interface CreatedSSOAccount {
  sessionIndex: number
  handoff: HandoffRequest | null
}

/**
 * createSSOAccount generates the account keys, confirms the SSO signup, logs
 * in with the new entity key, and completes any stored handoff. onLoggingIn
 * fires once the account is registered and sign-in begins.
 */
function createSSOAccount(
  root: Root,
  nonce: string,
  username: string,
  pin: string,
  confirmPin: string,
  onLoggingIn: () => void,
): Promise<CreatedSSOAccount> {
  const wantsPin = pin.length > 0 || confirmPin.length > 0

  return withSpacewaveProvider(root, async (spacewave) => {
    const { entity, session } = await generateAuthKeypairs(spacewave)
    const wrappedEntityKey = wantsPin
      ? await wrapPemWithPin(spacewave, entity.pem, pin)
      : entity.custodiedPemBase64

    await spacewave.confirmSSO({
      nonce,
      username,
      wrappedEntityKey,
      entityPeerId: entity.peerId,
      sessionPeerId: session.peerId,
      pinWrapped: wantsPin,
    })
    onLoggingIn()
    const sessionIndex = await loginWithEntityPem(
      root,
      new TextEncoder().encode(entity.pem),
    )
    const handoff = await completeStoredHandoff(root, sessionIndex)
    return { sessionIndex, handoff }
  })
}

/**
 * useSSOSignupFlow runs the confirm-then-create steps for a pending SSO
 * signup. Failures return to the form or the error step.
 */
function useSSOSignupFlow(
  pendingState: PendingSSOState | null,
  fields: SignupFields,
) {
  const navigate = useNavigate()
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const [state, setState] = useState<SSOConfirmState>({ step: 'form' })
  const { username, pin, confirmPin, reportError } = fields

  const requestConfirm = useCallback(() => {
    if (!pendingState) {
      setState({ step: 'error', message: 'SSO session expired' })
      return
    }
    const invalid = validateSignup(username, pin, confirmPin)
    if (invalid) {
      reportError(invalid)
      return
    }
    setState({ step: 'confirm', username })
  }, [pendingState, username, pin, confirmPin, reportError])

  const cancelConfirm = useCallback(() => {
    setState({ step: 'form' })
  }, [])

  const createAccount = useCallback(async () => {
    if (!pendingState) {
      setState({ step: 'error', message: 'SSO session expired' })
      return
    }
    const invalid = validateSignup(username, pin, confirmPin)
    if (invalid) {
      setState({ step: 'form' })
      reportError(invalid)
      return
    }
    if (!root) {
      setState({ step: 'error', message: 'Not connected to server' })
      return
    }

    setState({ step: 'creating' })
    try {
      const { sessionIndex, handoff } = await createSSOAccount(
        root,
        pendingState.nonce,
        username,
        pin,
        confirmPin,
        () => setState({ step: 'logging_in' }),
      )
      clearPendingSSOState()
      if (handoff) {
        setState({ step: 'handoff_complete', request: handoff, sessionIndex })
        return
      }
      navigate({ path: `/u/${sessionIndex}` })
    } catch (err) {
      if (isUsernameTakenError(err)) {
        setState({ step: 'form' })
        reportError({ field: 'username', error: 'Username is already taken' })
        return
      }
      setState({
        step: 'error',
        message: getErrorMessage(err, 'Account creation failed'),
      })
    }
  }, [pendingState, username, pin, confirmPin, reportError, root, navigate])

  return { state, requestConfirm, cancelConfirm, createAccount }
}

interface SSORecoveryActionsProps {
  routeProvider: string
  onRestartDesktop: () => void
  onCancel: () => void
}

/** SSORecoveryActions offers restarting desktop sign-in and returning to login. */
function SSORecoveryActions({
  routeProvider,
  onRestartDesktop,
  onCancel,
}: SSORecoveryActionsProps) {
  return (
    <div className="flex w-full flex-col gap-2">
      {isDesktop && routeProvider && (
        <AuthPrimaryActionButton onClick={onRestartDesktop}>
          Restart sign-in
        </AuthPrimaryActionButton>
      )}
      <BackToLoginButton onClick={onCancel}>Back to login</BackToLoginButton>
    </div>
  )
}

/** BackToLoginButton is the secondary action that leaves the signup step. */
function BackToLoginButton({
  onClick,
  children,
}: {
  onClick: () => void
  children: ReactNode
}) {
  return (
    <AuthSecondaryActionButton
      onClick={onClick}
      className="hover:text-brand flex items-center justify-center gap-1.5"
    >
      <LuArrowLeft className="size-3" />
      {children}
    </AuthSecondaryActionButton>
  )
}

/** SSOExpiredView is shown when the pending SSO state is gone. */
function SSOExpiredView(props: SSORecoveryActionsProps) {
  return (
    <AuthScreenLayout
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h2 className="text-foreground text-lg font-semibold">
            Session expired
          </h2>
        </>
      }
    >
      <AuthStatusPanel
        icon={<></>}
        message="Your sign-in session has expired. Please try again."
      >
        <SSORecoveryActions {...props} />
      </AuthStatusPanel>
    </AuthScreenLayout>
  )
}

/** SSOErrorView shows why account creation failed. */
function SSOErrorView({
  message,
  ...actions
}: SSORecoveryActionsProps & { message: string }) {
  return (
    <AuthScreenLayout
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h2 className="text-foreground text-lg font-semibold">
            Account creation failed
          </h2>
        </>
      }
    >
      <div className="flex w-full flex-col items-center gap-4">
        <LoadingCard
          view={{
            state: 'error',
            title: 'Account creation failed',
            error: message,
          }}
        />
        <SSORecoveryActions {...actions} />
      </div>
    </AuthScreenLayout>
  )
}

const creatingSteps = [
  'Creating secure account keys',
  'Protecting your account credentials',
  'Registering your username',
]
const loggingInSteps = [
  'Unlocking your account key',
  'Preparing your account workspace',
  'Opening Spacewave',
]

/** SSOProgressView shows the account creation or sign-in progress card. */
function SSOProgressView({
  loggingIn,
  username,
  email,
}: {
  loggingIn: boolean
  username: string
  email: string
}) {
  return (
    <AuthScreenLayout
      alwaysShowIntro
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h2 className="text-foreground text-lg font-semibold">
            {loggingIn ? 'Signing in…' : 'Creating account…'}
          </h2>
          {email && <p className="text-foreground-alt text-sm">{email}</p>}
        </>
      }
    >
      <AuthProgressCard
        title={loggingIn ? 'Signing you in' : `Creating ${username}`}
        detail={
          loggingIn
            ? 'Opening your new encrypted session.'
            : 'Creating secure account keys and registering your account.'
        }
        steps={loggingIn ? loggingInSteps : creatingSteps}
      />
    </AuthScreenLayout>
  )
}

/** SSOUsernameField is the username input with its format hint or error. */
function SSOUsernameField({
  fields,
  onSubmit,
}: {
  fields: SignupFields
  onSubmit: () => void
}) {
  const { username, usernameError, handleUsernameChange } = fields
  const handleInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-foreground-alt text-xs select-none">Username</span>
      <input
        ref={handleInputRef}
        value={username}
        onChange={handleUsernameChange}
        placeholder="your-name"
        className={cn(
          authInputClassName,
          usernameError && 'border-destructive/50',
        )}
        onKeyDown={(event) => {
          if (event.nativeEvent.isComposing) return
          if (event.key === 'Enter') {
            onSubmit()
          }
        }}
      />
      {usernameError ? (
        <p className="text-destructive text-xs">{usernameError}</p>
      ) : (
        <p className="text-foreground-alt/50 text-xs">
          Lowercase letters, numbers, and hyphens
        </p>
      )}
    </label>
  )
}

interface SSOSignupFormProps {
  pendingState: PendingSSOState
  fields: SignupFields
  onRequestConfirm: () => void
  onCancel: () => void
}

/** SSOSignupForm collects the username and optional PIN for a new SSO account. */
function SSOSignupForm({
  pendingState,
  fields,
  onRequestConfirm,
  onCancel,
}: SSOSignupFormProps) {
  const { username, usernameError, pin, confirmPin, pinError } = fields

  return (
    <div className="flex flex-col gap-4">
      <AuthCard>
        {/* Provider context header */}
        <div className="mb-4 flex items-center gap-3">
          <div className="bg-brand/10 flex size-10 items-center justify-center rounded-lg">
            <ProviderIcon provider={pendingState.provider} className="size-5" />
          </div>
          <div>
            <h2 className="text-foreground text-sm font-semibold">
              Sign up with {getProviderLabel(pendingState.provider)}
            </h2>
            {pendingState.email && (
              <p className="text-foreground-alt text-xs">
                {pendingState.email}
              </p>
            )}
          </div>
        </div>

        <div className="flex flex-col gap-4">
          <SSOUsernameField fields={fields} onSubmit={onRequestConfirm} />
          <OptionalPinLock
            pin={pin}
            confirmPin={confirmPin}
            pinError={pinError}
            onPinChange={fields.handlePinChange}
            onConfirmPinChange={fields.handleConfirmPinChange}
            onSubmit={onRequestConfirm}
            disabled={false}
            pinInputId="sso-pin"
          />
          <AuthPrimaryActionButton
            onClick={onRequestConfirm}
            disabled={!username || !!usernameError}
            icon={<LuUserPlus className="text-foreground size-4" />}
          >
            Create account
          </AuthPrimaryActionButton>
          <BackToLoginButton onClick={onCancel}>
            Back to login
          </BackToLoginButton>
        </div>
      </AuthCard>

      {/* Trust signals */}
      <div className="text-foreground-alt flex flex-wrap items-center justify-center gap-x-6 gap-y-1 text-xs">
        {SIGNUP_HIGHLIGHTS.map((text) => (
          <span key={text} className="flex items-center gap-1.5">
            <LuCheck className="text-brand size-3.5" />
            {text}
          </span>
        ))}
      </div>
    </div>
  )
}

interface ConfirmUsernameDialogProps {
  open: boolean
  username: string
  onConfirm: () => void
  onCancel: () => void
}

/** ConfirmUsernameDialog asks the user to confirm the permanent username. */
function ConfirmUsernameDialog({
  open,
  username,
  onConfirm,
  onCancel,
}: ConfirmUsernameDialogProps) {
  const tosHref = useStaticHref('/tos')
  const privacyHref = useStaticHref('/privacy')

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onCancel()
      }}
    >
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>Confirm your username</DialogTitle>
          <DialogDescription>
            Your account will be created as{' '}
            <span className="text-foreground font-semibold">{username}</span>.
            This username is permanent and cannot be changed later.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-2">
          <AuthPrimaryActionButton
            onClick={onConfirm}
            icon={<LuUserPlus className="text-foreground size-4" />}
          >
            Confirm and create account
          </AuthPrimaryActionButton>
          <p className="text-foreground-alt/70 text-center text-xs">
            By clicking Confirm, you agree to our{' '}
            <a
              href={tosHref}
              target="_blank"
              rel="noopener noreferrer"
              className="text-brand hover:underline"
            >
              Terms of Service
            </a>{' '}
            and{' '}
            <a
              href={privacyHref}
              target="_blank"
              rel="noopener noreferrer"
              className="text-brand hover:underline"
            >
              Privacy Policy
            </a>
            .
          </p>
          <BackToLoginButton onClick={onCancel}>
            Back to edit username
          </BackToLoginButton>
        </div>
      </DialogContent>
    </Dialog>
  )
}

// SSOConfirmPage handles new-account username entry after SSO.
// Route: /auth/sso/:provider/confirm
// Shared by both desktop and web SSO flows.
export function SSOConfirmPage() {
  const navigate = useNavigate()
  const params = useParams()
  const routeProvider = params?.provider ?? ''
  const pendingState = useMemo(() => getPendingSSOState(), [])
  const fields = useSignupFields()
  const { state, requestConfirm, cancelConfirm, createAccount } =
    useSSOSignupFlow(pendingState, fields)

  const handleCancel = useCallback(() => {
    clearPendingSSOState()
    navigate({ path: getAuthReturnPath() })
  }, [navigate])

  const handleRestartDesktop = useCallback(() => {
    clearPendingSSOState()
    navigate({ path: `/auth/sso/${routeProvider}` })
  }, [navigate, routeProvider])

  const recovery = {
    routeProvider,
    onRestartDesktop: handleRestartDesktop,
    onCancel: handleCancel,
  }

  if (state.step === 'handoff_complete') {
    return (
      <HandoffComplete
        request={state.request}
        sessionIndex={state.sessionIndex}
      />
    )
  }

  // No pending state = expired.
  if (!pendingState) return <SSOExpiredView {...recovery} />

  if (state.step === 'error') {
    return <SSOErrorView message={state.message} {...recovery} />
  }

  if (state.step === 'creating' || state.step === 'logging_in') {
    return (
      <SSOProgressView
        loggingIn={state.step === 'logging_in'}
        username={fields.username}
        email={pendingState.email}
      />
    )
  }

  // Form state (with optional confirm modal overlay).
  return (
    <AuthScreenLayout
      alwaysShowIntro
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h2 className="text-foreground text-lg font-semibold">
            Welcome to Spacewave
          </h2>
          <p className="text-foreground-alt text-sm">
            Choose a username to finish signing up
          </p>
        </>
      }
    >
      <SSOSignupForm
        pendingState={pendingState}
        fields={fields}
        onRequestConfirm={requestConfirm}
        onCancel={handleCancel}
      />
      <ConfirmUsernameDialog
        open={state.step === 'confirm'}
        username={state.step === 'confirm' ? state.username : fields.username}
        onConfirm={() => void createAccount()}
        onCancel={cancelConfirm}
      />
    </AuthScreenLayout>
  )
}
