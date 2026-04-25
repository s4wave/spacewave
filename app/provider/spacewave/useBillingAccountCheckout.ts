import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { CheckoutStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { getBrowserCheckoutResultBaseUrl } from './checkout-url.js'
import { useCloudProviderConfig } from './useSpacewaveAuth.js'

export interface UseBillingAccountCheckoutOptions {
  onCompleted?: () => void
}

// useBillingAccountCheckout creates and monitors a checkout session for one BA.
export function useBillingAccountCheckout(
  opts: UseBillingAccountCheckoutOptions = {},
) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const cloudProviderConfig = useCloudProviderConfig()
  const checkoutResultBaseUrl =
    getBrowserCheckoutResultBaseUrl(cloudProviderConfig)
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const [polling, setPolling] = useState(false)
  const [showRetry, setShowRetry] = useState(false)
  const [checkoutUrl, setCheckoutUrl] = useState('')
  const [error, setError] = useState<string | null>(null)

  const handleCompleted = useEffectEvent(() => {
    setPolling(false)
    setShowRetry(false)
    setCheckoutUrl('')
    setError(null)
    opts.onCompleted?.()
  })

  const checkoutStatusResource = useStreamingResource(
    sessionResource,
    useCallback(
      (sess: NonNullable<Session>, signal: AbortSignal) => {
        if (!polling) return (async function* () {})()
        return sess.spacewave.watchCheckoutStatus(signal)
      },
      [polling],
    ),
    [polling],
  )

  const checkoutStatus = checkoutStatusResource.value?.status
  useEffect(() => {
    if (checkoutStatus === CheckoutStatus.CheckoutStatus_COMPLETED) {
      handleCompleted()
      return
    }
    if (
      checkoutStatus === CheckoutStatus.CheckoutStatus_CANCELED ||
      checkoutStatus === CheckoutStatus.CheckoutStatus_EXPIRED
    ) {
      setPolling(false)
      setShowRetry(false)
      setCheckoutUrl('')
      setError('Checkout was not completed. You can try again.')
    }
  }, [checkoutStatus, handleCompleted])

  useEffect(() => {
    return () => {
      if (retryTimerRef.current) clearTimeout(retryTimerRef.current)
    }
  }, [])

  const startCheckout = useCallback(
    async (billingAccountId: string) => {
      if (!session || !checkoutResultBaseUrl) {
        setError('Billing checkout is unavailable.')
        return false
      }

      if (retryTimerRef.current) clearTimeout(retryTimerRef.current)
      setError(null)
      setShowRetry(false)
      setCheckoutUrl('')
      setPolling(false)

      try {
        const successUrl = checkoutResultBaseUrl + '/checkout/success'
        const cancelUrl = checkoutResultBaseUrl + '/checkout/cancel'
        const resp = await session.spacewave.createCheckoutSession({
          billingAccountId,
          successUrl,
          cancelUrl,
        })

        if (resp.status === CheckoutStatus.CheckoutStatus_COMPLETED) {
          handleCompleted()
          return true
        }

        const url = resp.checkoutUrl ?? ''
        if (url) {
          const win = window.open(url, '_blank')
          setCheckoutUrl(url)
          setPolling(true)
          if (!win) {
            setShowRetry(true)
            return false
          }
        }
        setPolling(true)
        retryTimerRef.current = setTimeout(() => setShowRetry(true), 4000)
        return true
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
        return false
      }
    },
    [checkoutResultBaseUrl, handleCompleted, session],
  )

  const continueCheckout = useCallback(() => {
    if (!checkoutUrl) return
    const win = window.open(checkoutUrl, '_blank')
    if (win) setShowRetry(false)
  }, [checkoutUrl])

  return {
    continueCheckout,
    error,
    polling,
    showRetry,
    startCheckout,
  }
}
