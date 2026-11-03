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
    display: 'flex',
    height: '100%',
    minHeight: 0,
    width: '100%',
    flexDirection: 'column',
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule2: {
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 14)`,
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-secondary']} 60%, transparent)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 5)`,
    },
  },
  rule3: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: 'var(--radius-sm)',
    backgroundColor: themeColors['--color-background-tertiary'],
    color: themeColors['--color-brand'],
  },
  rule4: { minWidth: 0 },
  rule5: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule6: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
  rule7: {
    display: 'flex',
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule8: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule9: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    animationName: 'spin',
    animationDuration: '1s',
    animationTimingFunction: 'linear',
    animationIterationCount: 'infinite',
    animationDirection: 'normal',
    animationPlayState: 'running',
    animationDelay: '0s',
    animationFillMode: 'none',
    animationTimeline: 'auto',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '2px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderTopColor: themeColors['--color-brand'],
  },
  rule10: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule11: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
  rule12: {
    maxWidth: 'var(--container-sm)',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-error']} 30%, transparent)`,
    backgroundColor: themeColors['--color-background-card'],
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    textAlign: 'center',
  },
  rule13: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule14: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `calc(${themeLayout['--spacing']} * 5)`,
    '--tw-leading': `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-muted-foreground'],
  },
  rule15: {
    marginInline: 'auto',
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: 'var(--radius-sm)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
    backgroundColor: {
      default: themeColors['--color-background-tertiary'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-background-panel'],
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-primary'],
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    marginInline: 'auto',
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    color: themeColors['--color-error-text'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
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
