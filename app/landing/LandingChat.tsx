import { LuHash, LuLayoutGrid, LuMessageSquare } from 'react-icons/lu'

import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'

import { UseCaseDemo } from './use-case/UseCaseDemo.js'
import { UseCasePage } from './use-case/UseCasePage.js'

export const metadata = {
  title: 'Spacewave Chat - Talk in a Space you own.',
  description:
    'Try a real Spacewave chat channel in your browser. Messages live in a private Space, encrypted end to end and synced to the devices and people you invite.',
  canonicalPath: '/landing/chat',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

// LandingChat renders the Chat use-case page. live mounts the demo channel.
export function LandingChat({ live = false }: { live?: boolean }) {
  const landingHref = useStaticHref('/landing')

  return (
    <UseCasePage
      icon={<LuMessageSquare className="size-8" />}
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
        live={live}
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
    <div className="flex h-full text-sm">
      <div className="border-foreground/10 flex w-44 shrink-0 flex-col gap-1 border-r p-3">
        <span className="text-foreground-alt text-metadata px-2 pb-1 font-semibold tracking-wide uppercase">
          My Chat
        </span>
        <span className="bg-foreground/5 text-foreground flex items-center gap-2 rounded px-2 py-1">
          <LuHash className="text-foreground-alt size-4" />
          general
        </span>
      </div>
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex flex-1 flex-col items-center justify-center gap-1 p-6 text-center">
          <span className="text-foreground font-medium">
            Start the conversation
          </span>
          <span className="text-foreground-alt max-w-xs text-xs">
            This channel is ready. Send the first message to everyone in the
            peer group.
          </span>
        </div>
        <div className="border-foreground/10 flex items-center gap-2 border-t p-3">
          <span className="border-foreground/10 text-foreground-alt flex-1 rounded border px-3 py-2">
            Message this channel
          </span>
          <span className="bg-brand text-background rounded px-3 py-2 text-xs font-medium">
            Send
          </span>
        </div>
      </div>
    </div>
  )
}
