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
    overscrollBehavior: 'contain',
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 5)`,
    },
    overflowY: 'auto',
  },
  rule2: {
    marginInline: 'auto',
    paddingBlock: `calc(${themeLayout['--spacing']} * 12)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    textAlign: 'center',
    height: '100%',
    maxWidth: 'var(--container-sm)',
  },
  rule3: {
    borderRadius: '3.40282e38px',
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-tertiary'],
    color: themeColors['--color-brand'],
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 11)`,
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 11)`,
  },
  rule4: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule5: {
    '--tw-leading': `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `calc(${themeLayout['--spacing']} * 5)`,
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule6: {
    gap: themeLayout['--spacing'],
    marginInline: 'auto',
    display: 'flex',
    flexDirection: 'column',
    maxWidth: 'var(--container-3xl)',
    width: '100%',
  },
  rule7: {
    paddingLeft: `calc(${themeLayout['--spacing']} * 11)`,
    paddingTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    paddingTop: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      ':first-child': 0,
    },
  },
  rule9: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    borderRadius: '3.40282e38px',
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-tertiary'],
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule10: { flex: 1, minWidth: 0 },
  rule11: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'baseline',
    display: 'flex',
    marginBottom: `calc(${themeLayout['--spacing']} * .5)`,
    minWidth: 0,
  },
  rule12: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    overflow: 'hidden',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule13: {
    color: themeColors['--color-muted-foreground'],
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule14: {
    '--tw-leading': `calc(${themeLayout['--spacing']} * 6)`,
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `calc(${themeLayout['--spacing']} * 6)`,
    overflowWrap: 'break-word',
    whiteSpace: 'pre-wrap',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
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
