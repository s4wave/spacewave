import { useCallback, useEffect, useState } from 'react'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import type { EmailInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'

// EmailManagement captures the reactive email list and the full mutation
// surface shared between the onboarding VerifyEmailPage and the post-onboarding
// EmailSection. All handlers route errors through toast.error and success
// through toast.success; mutation state is exposed per action so callers can
// disable specific buttons without conflating global busy flags.
export interface EmailManagement {
  emails: EmailInfo[] | null
  loading: boolean

  verifyingEmail: string | null
  setVerifyingEmail: (email: string | null) => void
  code: string
  setCode: (code: string) => void
  retryAfter: number

  sendingCode: string | null
  verifyingCode: boolean
  addingEmail: boolean
  removingEmail: string | null
  settingPrimary: string | null

  sendCode: (email: string) => Promise<boolean>
  verifyCode: () => Promise<boolean>
  addEmail: (email: string) => Promise<boolean>
  removeEmail: (email: string) => Promise<boolean>
  setPrimaryEmail: (email: string) => Promise<boolean>
}

// useEmailManagement streams the account's email list and returns the full
// mutation surface needed by any email-management UI.
export function useEmailManagement(): EmailManagement {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)

  const emailsResource = useStreamingResource(
    sessionResource,
    useCallback(
      (session: NonNullable<Session>, signal: AbortSignal) =>
        session.spacewave.watchEmails(signal),
      [],
    ),
    [],
  )
  const emails = emailsResource.value?.emails ?? null
  const loading = emailsResource.loading

  const [verifyingEmail, setVerifyingEmail] = useState<string | null>(null)
  const [code, setCode] = useState('')
  const [retryAfter, setRetryAfter] = useState(0)
  const [sendingCode, setSendingCode] = useState<string | null>(null)
  const [verifyingCode, setVerifyingCode] = useState(false)
  const [addingEmail, setAddingEmail] = useState(false)
  const [removingEmail, setRemovingEmail] = useState<string | null>(null)
  const [settingPrimary, setSettingPrimary] = useState<string | null>(null)

  useEffect(() => {
    if (retryAfter <= 0) return
    const id = setTimeout(() => setRetryAfter((v) => v - 1), 1000)
    return () => clearTimeout(id)
  }, [retryAfter])

  // Auto-clear the verify flow when the watched row becomes verified
  // elsewhere; keeps stale forms from lingering across tabs or sessions.
  useEffect(() => {
    if (!verifyingEmail || !emails) return
    const row = emails.find((e) => e.email === verifyingEmail)
    if (row?.verified) {
      setVerifyingEmail(null)
      setCode('')
    }
  }, [emails, verifyingEmail])

  const sendCode = useCallback(
    async (email: string): Promise<boolean> => {
      if (!session) return false
      setSendingCode(email)
      try {
        const resp = await session.spacewave.sendVerificationEmail(email)
        setVerifyingEmail(email)
        setCode('')
        if (resp.retryAfter) {
          setRetryAfter(resp.retryAfter)
          toast.success(
            'Verification code sent. You can resend in ' +
              resp.retryAfter +
              's.',
          )
        } else {
          toast.success('Verification code sent to ' + email)
        }
        return true
      } catch (err) {
        const msg =
          err instanceof Error ?
            err.message
          : 'Failed to send verification email'
        toast.error(msg)
        return false
      } finally {
        setSendingCode(null)
      }
    },
    [session],
  )

  const verifyCode = useCallback(async (): Promise<boolean> => {
    if (!session || !verifyingEmail || !code) return false
    setVerifyingCode(true)
    try {
      await session.spacewave.verifyEmailCode(verifyingEmail, code)
      toast.success('Email verified')
      setVerifyingEmail(null)
      setCode('')
      return true
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Invalid or expired code'
      toast.error(msg)
      return false
    } finally {
      setVerifyingCode(false)
    }
  }, [session, verifyingEmail, code])

  const addEmail = useCallback(
    async (email: string): Promise<boolean> => {
      if (!session || !email) return false
      setAddingEmail(true)
      try {
        const resp = await session.spacewave.addEmail(email)
        setVerifyingEmail(email)
        setCode('')
        if (resp.retryAfter) {
          setRetryAfter(resp.retryAfter)
          toast.success(
            'Email added. You can send a code in ' + resp.retryAfter + 's.',
          )
        } else {
          toast.success('Verification code sent to ' + email)
        }
        return true
      } catch (err) {
        const msg = err instanceof Error ? err.message : 'Failed to add email'
        toast.error(msg)
        return false
      } finally {
        setAddingEmail(false)
      }
    },
    [session],
  )

  const removeEmail = useCallback(
    async (email: string): Promise<boolean> => {
      if (!session) return false
      setRemovingEmail(email)
      try {
        await session.spacewave.removeEmail(email)
        toast.success('Email removed')
        if (verifyingEmail === email) {
          setVerifyingEmail(null)
          setCode('')
        }
        return true
      } catch (err) {
        const msg =
          err instanceof Error ? err.message : 'Cannot remove this email'
        toast.error(msg)
        return false
      } finally {
        setRemovingEmail(null)
      }
    },
    [session, verifyingEmail],
  )

  const setPrimaryEmail = useCallback(
    async (email: string): Promise<boolean> => {
      if (!session) return false
      setSettingPrimary(email)
      try {
        await session.spacewave.setPrimaryEmail(email)
        toast.success('Primary email updated to ' + email)
        return true
      } catch (err) {
        const msg =
          err instanceof Error ? err.message : 'Failed to set primary email'
        toast.error(msg)
        return false
      } finally {
        setSettingPrimary(null)
      }
    },
    [session],
  )

  return {
    emails,
    loading,
    verifyingEmail,
    setVerifyingEmail,
    code,
    setCode,
    retryAfter,
    sendingCode,
    verifyingCode,
    addingEmail,
    removingEmail,
    settingPrimary,
    sendCode,
    verifyCode,
    addEmail,
    removeEmail,
    setPrimaryEmail,
  }
}
