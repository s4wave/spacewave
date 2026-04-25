import type { CloudProviderConfig } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

function trimTrailingSlash(url: string): string {
  return url.replace(/\/+$/, '')
}

// getCheckoutResultBaseUrl returns the public browser URL used for Stripe redirects.
export function getCheckoutResultBaseUrl(
  cloudProviderConfig: CloudProviderConfig | null,
): string {
  const accountBaseUrl = cloudProviderConfig?.accountBaseUrl?.trim() ?? ''
  if (accountBaseUrl) return trimTrailingSlash(accountBaseUrl)

  const publicBaseUrl = cloudProviderConfig?.publicBaseUrl?.trim() ?? ''
  if (publicBaseUrl) return trimTrailingSlash(publicBaseUrl)

  return ''
}

// getBrowserCheckoutResultBaseUrl returns a browser-safe checkout redirect base.
export function getBrowserCheckoutResultBaseUrl(
  cloudProviderConfig: CloudProviderConfig | null,
): string {
  const configured = getCheckoutResultBaseUrl(cloudProviderConfig)
  if (configured) return configured
  if (typeof window === 'undefined') return ''
  return trimTrailingSlash(window.location.origin)
}
