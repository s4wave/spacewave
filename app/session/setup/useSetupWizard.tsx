import { useCallback, useState } from 'react'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { downloadPemFile } from '@s4wave/web/download.js'
import { useMountAccount } from '@s4wave/web/hooks/useMountAccount.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SessionLockMode } from '@s4wave/core/session/session.pb.js'
import { useSessionOnboardingState } from '@s4wave/app/session/setup/LocalSessionOnboardingContext.js'

// SetupWizardState is the return type of useSetupWizard.
export type SetupWizardState = ReturnType<typeof useSetupWizard>

// useSetupWizard provides shared state and handlers for the setup wizard.
// Used by both CloudSetupWizard and LocalSetupWizard.
export function useSetupWizard() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const [lockMode, setLockMode] = useState<'auto' | 'pin'>('auto')
  const [pin, setPin] = useState('')
  const [confirmPin, setConfirmPin] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [downloading, setDownloading] = useState(false)

  const { providerId, accountId, isCloud } = useSessionInfo(session)
  const onboarding = useSessionOnboardingState()

  // Mount account resource for cloud sessions (GenerateBackupKey RPC).
  // Local sessions use session.exportBackupKey instead.
  const accountResource = useMountAccount(providerId, accountId, isCloud)

  const accountReady = isCloud ? !!accountResource.value : !!session

  const handleDownloadPem = useCallback(
    async (pemCredential?: Uint8Array) => {
      if (!password && !pemCredential) {
        setError('Password or backup key is required')
        return
      }
      setDownloading(true)
      setError(null)
      try {
        let pemData: Uint8Array | undefined

        if (isCloud) {
          // Cloud: use account resource GenerateBackupKey RPC.
          const acct = accountResource.value
          if (!acct) {
            setError('Account not ready')
            return
          }
          const credential =
            pemCredential ?
              {
                credential: {
                  case: 'pemPrivateKey' as const,
                  value: pemCredential,
                },
              }
            : { credential: { case: 'password' as const, value: password } }
          const resp = await acct.generateBackupKey({ credential })
          pemData = resp.pemData
        } else {
          // Local: use session ExportBackupKey RPC.
          if (!session) {
            setError('Session not ready')
            return
          }
          const resp = await session.localProvider.exportBackupKey({ password })
          pemData = resp.pemData
        }

        if (!pemData || pemData.length === 0) {
          setError('No PEM data returned')
          return
        }

        downloadPemFile(pemData)

        onboarding.markBackupComplete()
        return true
      } catch (err) {
        const msg =
          err instanceof Error ? err.message : 'Failed to generate backup key'
        setError(msg)
        return false
      } finally {
        setDownloading(false)
      }
    },
    [isCloud, accountResource, session, password, onboarding],
  )

  const handleSkipPem = useCallback(() => {
    onboarding.markBackupComplete()
  }, [onboarding])

  const handleFinishLock = useCallback(async () => {
    if (lockMode === 'pin') {
      if (pin.length < 4) {
        setError('PIN must be at least 4 digits')
        return
      }
      if (pin !== confirmPin) {
        setError('PINs do not match')
        return
      }
    }
    setError(null)
    setSaving(true)
    try {
      if (session) {
        const mode =
          lockMode === 'pin' ?
            SessionLockMode.PIN_ENCRYPTED
          : SessionLockMode.AUTO_UNLOCK
        const pinBytes =
          lockMode === 'pin' ? new TextEncoder().encode(pin) : undefined
        await session.setLockMode(mode, pinBytes)
      }
      onboarding.markLockComplete()
      setSaving(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to set lock mode')
      setSaving(false)
    }
  }, [session, lockMode, pin, confirmPin, onboarding])

  const handleSkipLock = useCallback(() => {
    onboarding.markLockComplete()
  }, [onboarding])

  return {
    providerId,
    accountReady,
    backupComplete: onboarding.onboarding.backupComplete,
    lockComplete: onboarding.onboarding.lockComplete,
    lockMode,
    setLockMode,
    pin,
    setPin,
    confirmPin,
    setConfirmPin,
    password,
    setPassword,
    error,
    setError,
    saving,
    downloading,
    handleDownloadPem,
    handleSkipPem,
    handleFinishLock,
    handleSkipLock,
  }
}
