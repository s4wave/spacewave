import { keyframeNames } from '../../web/style/tokens.stylex.js'
import { LuCircleAlert, LuHash, LuRefreshCw } from '@s4wave/web/images/icons.js'
import { useCallback, useEffect, useState } from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'

import { ChatHandle, ChatChannelTypeID } from '@s4wave/sdk/chat/chat.js'
import type { ChatMessageInfo } from '@s4wave/sdk/chat/rpc/rpc.pb.js'
import { MessageInput } from './MessageInput.js'
import { MessageList } from './MessageList.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    minHeight: 0,
    width: '100%',
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 5)`,
    },
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-secondary']} 60%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    minHeight: `calc(${themeLayout['--spacing']} * 14)`,
  },
  rule3: {
    borderRadius: 'var(--radius-sm)',
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-tertiary'],
    color: themeColors['--color-brand'],
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule4: { minWidth: 0 },
  rule5: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule6: {
    overflow: 'hidden',
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule7: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    flex: 1,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
  },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule9: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '2px',
    animationDelay: '0s',
    animationDirection: 'normal',
    animationDuration: '1s',
    animationFillMode: 'none',
    animationIterationCount: 'infinite',
    animationName: keyframeNames['spin'],
    animationPlayState: 'running',
    animationTimeline: 'auto',
    animationTimingFunction: 'linear',
    borderTopColor: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule10: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule11: {
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule12: {
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-error']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: themeColors['--color-background-card'],
    textAlign: 'center',
    maxWidth: 'var(--container-sm)',
  },
  rule13: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule14: {
    '--tw-leading': `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `calc(${themeLayout['--spacing']} * 5)`,
    marginTop: themeLayout['--spacing'],
  },
  rule15: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-primary'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
    borderRadius: 'var(--radius-sm)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    marginInline: 'auto',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: themeColors['--color-background-tertiary'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-background-panel'],
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    marginInline: 'auto',
    color: themeColors['--color-error-text'],
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export { ChatChannelTypeID }

// ChatChannelViewer displays live channel history and its composer.
export function ChatChannelViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  return (
    <ChatChannelContent
      key={objectKey}
      objectKey={objectKey}
      worldState={worldState}
    />
  )
}

function ChatChannelContent({
  objectKey,
  worldState,
}: {
  objectKey: string
  worldState: ObjectViewerComponentProps['worldState']
}) {
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    ChatHandle,
    ChatChannelTypeID,
  )
  const [messages, setMessages] = useState<ChatMessageInfo[]>([])

  const streamFactory = useCallback((chat: ChatHandle, signal: AbortSignal) => {
    setMessages([])
    return chat.watchMessages(signal)
  }, [])
  const messagesResource = useStreamingResource(handle, streamFactory, [])
  const infoResource = useStreamingResource(
    handle,
    (chat: ChatHandle, signal: AbortSignal) => chat.watchChannelInfo(signal),
    [],
  )
  useObjectName(infoResource.value?.name)

  useEffect(() => {
    const batch = messagesResource.value
    if (messagesResource.loading || !batch?.length) return
    setMessages((current) => {
      const keys = new Set(current.map((message) => message.objectKey))
      const additions = batch.filter((message) => !keys.has(message.objectKey))
      return additions.length === 0 ? current : [...current, ...additions]
    })
  }, [messagesResource.loading, messagesResource.value])

  const handleSend = useCallback(
    async (text: string) => {
      const chat = handle.value
      if (!chat) throw new Error('The channel is not connected yet.')
      await chat.sendMessage(text)
    },
    [handle.value],
  )

  const loading = messagesResource.loading && messages.length === 0
  const error = messagesResource.error

  return (
    <section sx={stylexStyles2.rule1} aria-label="Chat channel">
      <header sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <LuHash sx={stylexStyles.rule1} aria-hidden="true" />
        </div>
        <div sx={stylexStyles2.rule4}>
          <h1 sx={stylexStyles2.rule5}>Chat channel</h1>
          <p sx={stylexStyles2.rule6}>{objectKey || 'Conversation'}</p>
        </div>
      </header>

      {loading ? (
        <div sx={stylexStyles2.rule7} role="status">
          <div sx={stylexStyles2.rule8}>
            <div sx={stylexStyles2.rule9} />
            <div>
              <div sx={stylexStyles2.rule10}>Loading messages</div>
              <div sx={stylexStyles2.rule11}>Reading conversation history…</div>
            </div>
          </div>
        </div>
      ) : error ? (
        <div sx={stylexStyles2.rule7} role="alert">
          <div sx={stylexStyles2.rule12}>
            <LuCircleAlert sx={stylexStyles.rule2} aria-hidden="true" />
            <h2 sx={stylexStyles2.rule13}>Messages unavailable</h2>
            <p sx={stylexStyles2.rule14}>
              The channel history could not be read. Check the connection and
              try again.
            </p>
            <button
              type="button"
              onClick={messagesResource.retry}
              sx={stylexStyles2.rule15}
            >
              <LuRefreshCw sx={stylexStyles.rule3} aria-hidden="true" />
              Try again
            </button>
          </div>
        </div>
      ) : (
        <MessageList messages={messages} />
      )}

      <MessageInput
        disabled={loading || Boolean(error) || !handle.value}
        onSend={handleSend}
      />
    </section>
  )
}
