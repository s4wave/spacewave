import { LuMessageCircle } from '@s4wave/web/images/icons.js'
import { useCallback, useEffect, useRef } from 'react'

import type { ChatMessageInfo } from '@s4wave/sdk/chat/rpc/rpc.pb.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    flex: 1,
    overflowY: 'auto',
    overscrollBehavior: 'contain',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 5)`,
    },
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    marginInline: 'auto',
    display: 'flex',
    height: '100%',
    maxWidth: 'var(--container-sm)',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 12)`,
    textAlign: 'center',
  },
  rule3: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 11)`,
    height: `calc(${themeLayout['--spacing']} * 11)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-background-tertiary'],
    color: themeColors['--color-brand'],
  },
  rule4: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule5: {
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `calc(${themeLayout['--spacing']} * 5)`,
    '--tw-leading': `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-muted-foreground'],
  },
  rule6: {
    marginInline: 'auto',
    display: 'flex',
    width: '100%',
    maxWidth: 'var(--container-3xl)',
    flexDirection: 'column',
    gap: themeLayout['--spacing'],
  },
  rule7: {
    paddingTop: `calc(${themeLayout['--spacing']} * .5)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 11)`,
  },
  rule8: {
    display: 'flex',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingTop: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      ':first-child': 0,
    },
  },
  rule9: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-background-tertiary'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
  },
  rule10: { minWidth: 0, flex: 1 },
  rule11: {
    marginBottom: `calc(${themeLayout['--spacing']} * .5)`,
    display: 'flex',
    minWidth: 0,
    alignItems: 'baseline',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule12: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground-alt'],
  },
  rule13: {
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
  rule14: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `calc(${themeLayout['--spacing']} * 6)`,
    '--tw-leading': `calc(${themeLayout['--spacing']} * 6)`,
    overflowWrap: 'break-word',
    whiteSpace: 'pre-wrap',
    color: themeColors['--color-foreground'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

function peerLabel(id: string): string {
  if (!id) return 'Unknown peer'
  if (id.length <= 14) return id
  return `${id.slice(0, 7)}…${id.slice(-5)}`
}

const timeFormatter = new Intl.DateTimeFormat(undefined, {
  hour: 'numeric',
  minute: '2-digit',
})

function formatTime(date: Date | undefined): string {
  return date ? timeFormatter.format(date) : ''
}

// MessageList renders channel history and follows new messages while the reader remains at the bottom.
export function MessageList({ messages }: { messages: ChatMessageInfo[] }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const wasAtBottomRef = useRef(true)

  useEffect(() => {
    if (wasAtBottomRef.current && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight
    }
  }, [messages.length])

  const handleScroll = useCallback(() => {
    const element = containerRef.current
    if (!element) return
    wasAtBottomRef.current =
      element.scrollHeight - element.scrollTop - element.clientHeight < 40
  }, [])

  return (
    <div
      ref={containerRef}
      onScroll={handleScroll}
      aria-label="Channel messages"
      sx={stylexStyles2.rule1}
    >
      {messages.length === 0 ? (
        <div sx={stylexStyles2.rule2}>
          <div sx={stylexStyles2.rule3}>
            <LuMessageCircle sx={stylexStyles.rule1} aria-hidden="true" />
          </div>
          <h2 sx={stylexStyles2.rule4}>Start the conversation</h2>
          <p sx={stylexStyles2.rule5}>
            This channel is ready. Send the first message to everyone in the
            peer group.
          </p>
        </div>
      ) : (
        <div sx={stylexStyles2.rule6}>
          {messages.map((message, index) => {
            const previous = messages[index - 1]
            const grouped = previous?.senderPeerId === message.senderPeerId
            return (
              <article
                key={message.objectKey}
                sx={grouped ? stylexStyles2.rule7 : stylexStyles2.rule8}
              >
                {!grouped && (
                  <div sx={stylexStyles2.rule9}>
                    {(message.senderPeerId || '?').slice(0, 1)}
                  </div>
                )}
                <div sx={stylexStyles2.rule10}>
                  {!grouped && (
                    <div sx={stylexStyles2.rule11}>
                      <span sx={stylexStyles2.rule12}>
                        {peerLabel(message.senderPeerId ?? '')}
                      </span>
                      <time sx={stylexStyles2.rule13}>
                        {formatTime(message.createdAt)}
                      </time>
                    </div>
                  )}
                  <p sx={stylexStyles2.rule14}>{message.text ?? ''}</p>
                </div>
              </article>
            )
          })}
        </div>
      )}
    </div>
  )
}
