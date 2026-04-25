export type SSOFlowMode = 'link' | 'unlock'

export interface SSOFinishMessage {
  type: 'spacewave-sso-finish'
  mode: SSOFlowMode
  provider?: string
  code?: string
  error?: string
}

export interface StartSSOPopupFlowOptions {
  provider: string
  ssoBaseUrl: string
  origin: string
  mode: SSOFlowMode
}

export interface SSOPopupFlow {
  waitForResult: Promise<string>
  cancel: () => void
}

const ssoChannelPrefix = 'spacewave-sso'

function buildChannelName(id: string): string {
  return `${ssoChannelPrefix}:${id}`
}

export function startSSOPopupFlow(
  opts: StartSSOPopupFlowOptions,
): SSOPopupFlow {
  const channelID = crypto.randomUUID()
  const channel = new BroadcastChannel(buildChannelName(channelID))
  const redirectPath =
    `/auth/sso/link/${opts.provider}/finish?channel=` +
    encodeURIComponent(channelID)
  const url =
    `${opts.ssoBaseUrl}/${opts.provider}?origin=` +
    encodeURIComponent(opts.origin) +
    '&mode=' +
    encodeURIComponent(opts.mode) +
    '&redirect_path=' +
    encodeURIComponent(redirectPath)
  const popup = window.open(
    url,
    '_blank',
    'popup=yes,width=560,height=720,noopener,noreferrer',
  )
  if (!popup) {
    channel.close()
    throw new Error('Popup was blocked. Allow popups and try again.')
  }

  let settled = false
  let rejectWait: ((err: Error) => void) | null = null
  const cancel = () => {
    if (settled) {
      return
    }
    settled = true
    channel.close()
    popup.close()
    rejectWait?.(new Error('SSO flow canceled'))
  }

  const waitForResult = new Promise<string>((resolve, reject) => {
    rejectWait = reject
    channel.onmessage = (event: MessageEvent<SSOFinishMessage>) => {
      const data = event.data
      if (data?.type !== 'spacewave-sso-finish') {
        return
      }
      if (data.mode !== opts.mode) {
        return
      }
      if (data.provider && data.provider !== opts.provider) {
        return
      }
      settled = true
      channel.close()
      popup.close()
      if (data.error) {
        reject(new Error(data.error))
        return
      }
      if (!data.code) {
        reject(new Error('OAuth callback did not return a code'))
        return
      }
      resolve(data.code)
    }
  })

  return {
    waitForResult,
    cancel,
  }
}
