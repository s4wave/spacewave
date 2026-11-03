import { LuMessageCircle } from '@s4wave/web/images/icons.js'

import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { ChatMessage } from '@s4wave/sdk/chat/chat.pb.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    overflow: 'auto',
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule4: {
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule5: {
    flex: 1,
    overflow: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

export const ChatMessageTypeID = 'spacewave-chat/message'

// ChatMessageViewer displays a Chat Message entity.
export function ChatMessageViewer({ objectState }: ObjectViewerComponentProps) {
  const message = useForgeBlockData(objectState, ChatMessageTypeID, ChatMessage)

  const textContent =
    message?.content?.content?.case === 'text'
      ? message.content.content.value
      : undefined

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <LuMessageCircle sx={stylexStyles.rule1} />
          <span sx={stylexStyles2.rule4}>Message</span>
        </div>
      </div>
      <div sx={stylexStyles2.rule5}>
        <InfoCard>
          <div sx={false}>
            {message?.senderPeerId && (
              <CopyableField
                sx={nativeChildSpacing.s2}
                label="Sender Peer ID"
                value={message.senderPeerId}
              />
            )}
            {textContent && (
              <CopyableField
                sx={nativeChildSpacing.s2}
                label="Content"
                value={textContent}
              />
            )}
            {message?.createdAt && (
              <CopyableField
                sx={nativeChildSpacing.s2}
                label="Created At"
                value={message.createdAt.toISOString()}
              />
            )}
            {message?.replyToKey && (
              <CopyableField
                sx={nativeChildSpacing.s2}
                label="Reply To Key"
                value={message.replyToKey}
              />
            )}
          </div>
        </InfoCard>
      </div>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
