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

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: 'var(--text-lg)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
  },
  rule2: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: themeColors['--color-background-get-started'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    textAlign: 'center',
  },
  rule4: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule2: {
    color: themeColors['--color-destructive'],
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
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
          <h2 sx={stylexStyles3.rule1}>
            <ProviderIcon provider={provider} sx={stylexStyles2.rule1} />
            {ok ? 'Return to Spacewave' : 'SSO link failed'}
          </h2>
        </>
      }
    >
      <div sx={stylexStyles3.rule2}>
        <div sx={stylexStyles3.rule3}>
          {ok ? (
            <LuCheck sx={stylexStyles.rule1} />
          ) : (
            <LuCircleAlert sx={stylexStyles.rule2} />
          )}
          <p sx={stylexStyles3.rule4}>
            {ok
              ? 'This window can close. Finish confirming the account link in the original Spacewave window.'
              : 'The OAuth callback data was incomplete. Close this window and try linking again.'}
          </p>
        </div>
      </div>
    </AuthScreenLayout>
  )
}
