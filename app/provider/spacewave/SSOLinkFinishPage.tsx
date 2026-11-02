import { useEffect } from 'react'
import { LuCheck, LuCircleAlert } from '@s4wave/web/images/icons.js'
import { FcGoogle } from '@s4wave/web/images/icons.js'
import { LuGithub } from '@s4wave/web/images/icons.js'

import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useParams } from '@s4wave/web/router/router.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    color: themeColors['--color-brand'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    color: themeColors['--color-destructive'],
  },
})

interface LinkFinishPayload {
  channel: string
  code: string
  provider: string
  mode: 'link' | 'unlock'
}

function parseLinkFinishPayload(): LinkFinishPayload | null {
  const hash = window.location.hash
  const idx = hash.indexOf('?')
  if (idx === -1) {
    return null
  }
  const params = new URLSearchParams(hash.slice(idx))
  const channel = params.get('channel') ?? ''
  const code = params.get('code') ?? ''
  const provider = params.get('provider') ?? ''
  const mode = params.get('mode') === 'unlock' ? 'unlock' : 'link'
  if (!channel || !provider) {
    return null
  }
  return { channel, code, provider, mode }
}

function ProviderIcon({
  provider,
  sx,
}: {
  provider: string
  sx?: import('@stylexjs/stylex').StyleXStyles
}) {
  if (provider === 'google') {
    return <FcGoogle sx={sx} />
  }
  if (provider === 'github') {
    return <LuGithub sx={sx} />
  }
  return null
}

export function SSOLinkFinishPage() {
  const params = useParams()
  const provider = params?.provider ?? ''
  const payload = parseLinkFinishPayload()
  const ok = !!(
    payload?.channel &&
    payload.code &&
    payload.provider === provider
  )

  useEffect(() => {
    if (!payload?.channel) {
      return
    }
    const channel = new BroadcastChannel(`spacewave-sso:${payload.channel}`)
    channel.postMessage({
      type: 'spacewave-sso-finish',
      mode: payload.mode,
      provider: payload.provider,
      code: payload.code,
      error: ok ? undefined : 'SSO callback is missing provider data',
    })
    channel.close()
    window.close()
  }, [ok, payload])

  return (
    <AuthScreenLayout
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h2 className="text-foreground flex items-center gap-2 text-lg font-semibold">
            <ProviderIcon provider={provider} sx={stylexStyles2.rule1} />
            {ok ? 'Return to Spacewave' : 'SSO link failed'}
          </h2>
        </>
      }
    >
      <div className="border-foreground/20 bg-background-get-started rounded-lg border p-5 shadow-lg backdrop-blur-sm">
        <div className="flex flex-col items-center gap-3 text-center">
          {ok ? (
            <LuCheck sx={stylexStyles.rule1} />
          ) : (
            <LuCircleAlert sx={stylexStyles.rule2} />
          )}
          <p className="text-foreground-alt text-sm">
            {ok
              ? 'This window can close. Finish confirming the account link in the original Spacewave window.'
              : 'The OAuth callback data was incomplete. Close this window and try linking again.'}
          </p>
        </div>
      </div>
    </AuthScreenLayout>
  )
}
