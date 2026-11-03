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
    height: '100%',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule2: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 44)`,
    flexShrink: 0,
    flexDirection: 'column',
    gap: themeLayout['--spacing'],
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBottom: themeLayout['--spacing'],
    fontSize: themeLayout['--text-metadata'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    color: themeColors['--color-foreground'],
  },
  rule5: { display: 'flex', minWidth: 0, flex: 1, flexDirection: 'column' },
  rule6: {
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: themeLayout['--spacing'],
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    textAlign: 'center',
  },
  rule7: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule8: {
    maxWidth: 'var(--container-xs)',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule9: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule10: {
    flex: 1,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule11: {
    borderRadius: '.25rem',
    backgroundColor: themeColors['--color-brand'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-background'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
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
