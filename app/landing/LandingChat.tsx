import type { ComponentType } from 'react'
import {
  LuHash,
  LuLayoutGrid,
  LuMessageSquare,
} from '@s4wave/web/images/icons.js'

import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'

import type { LiveDemoAppProps } from './use-case/LiveDemoApp.js'
import { UseCaseDemo } from './use-case/UseCaseDemo.js'
import { UseCasePage } from './use-case/UseCasePage.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    height: '100%',
  },
  rule2: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    gap: themeLayout['--spacing'],
    display: 'flex',
    flexDirection: 'column',
    flexShrink: 0,
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    width: `calc(${themeLayout['--spacing']} * 44)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeLayout['--text-metadata'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    textTransform: 'uppercase',
    paddingBottom: themeLayout['--spacing'],
  },
  rule4: {
    borderRadius: '.25rem',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground'],
    display: 'flex',
  },
  rule5: { flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 },
  rule6: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    flex: 1,
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    textAlign: 'center',
  },
  rule7: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule8: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    maxWidth: 'var(--container-xs)',
  },
  rule9: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
  },
  rule10: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule11: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: themeColors['--color-brand'],
    color: themeColors['--color-background'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule2: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

export const metadata = {
  title: 'Spacewave Chat - Talk in a Space you own.',
  description:
    'Try a real Spacewave chat channel in your browser. Messages live in a private Space, encrypted end to end and synced to the devices and people you invite.',
  canonicalPath: '/landing/chat',
}

// LandingChat renders the Chat use-case page. liveApp mounts the demo channel.
export function LandingChat({
  liveApp,
}: {
  liveApp?: ComponentType<LiveDemoAppProps>
}) {
  const landingHref = useStaticHref('/landing')

  return (
    <UseCasePage
      icon={<LuMessageSquare sx={stylexStyles.rule1} />}
      title="Talk in a Space you own."
      subtitle="Spacewave Chat keeps channels inside a private Space. The history lives on your devices and the devices of the people you invite, not on a chat server."
      points={[
        {
          title: 'A channel is an object',
          body: 'Chat is one object in a Space, next to its files and notes. Everyone invited to the Space sees the same channels.',
        },
        {
          title: 'History on your devices',
          body: 'Each linked device keeps the full message history in local storage, so past conversations open without a connection.',
        },
        {
          title: 'Encrypted end to end',
          body: 'Messages are encrypted on your devices. Spacewave Cloud backup is optional and only stores data encrypted before upload.',
        },
      ]}
      keepTitle="Keep your channel"
      keepBody="The demo lives in memory and is gone when you leave. The Chat Quickstart creates the same Space in your browser's storage, where you can invite people to join it."
      primary={{
        href: '#/quickstart/chat',
        label: 'Start a chat',
        icon: LuMessageSquare,
      }}
      secondary={{
        href: landingHref,
        label: 'See all features',
        icon: LuLayoutGrid,
      }}
    >
      <UseCaseDemo
        demo="chat"
        liveApp={liveApp}
        label="Chat"
        poster={<ChatPoster />}
        suggestions={[
          'Type a message in general and press Send.',
          'Send a second message to watch the history grow.',
          'Press Reset to start over with an empty channel.',
        ]}
      />
    </UseCasePage>
  )
}

// ChatPoster previews the Chat Quickstart: an empty general channel.
function ChatPoster() {
  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <span sx={stylexStyles2.rule3}>My Chat</span>
        <span sx={stylexStyles2.rule4}>
          <LuHash sx={stylexStyles.rule2} />
          general
        </span>
      </div>
      <div sx={stylexStyles2.rule5}>
        <div sx={stylexStyles2.rule6}>
          <span sx={stylexStyles2.rule7}>Start the conversation</span>
          <span sx={stylexStyles2.rule8}>
            This channel is ready. Send the first message to everyone in the
            peer group.
          </span>
        </div>
        <div sx={stylexStyles2.rule9}>
          <span sx={stylexStyles2.rule10}>Message this channel</span>
          <span sx={stylexStyles2.rule11}>Send</span>
        </div>
      </div>
    </div>
  )
}
