import {
  LuCheck,
  LuGlobe,
  LuLock,
  LuMessageSquare,
  LuRocket,
  LuShield,
  LuSmartphone,
  LuUsers,
} from 'react-icons/lu'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { ChatLandingDemo } from './LandingDemos.js'
import { LegalPageLayout } from './LegalPageLayout.js'
import { UseCaseCallout } from './UseCaseCallout.js'
import { UseCaseCtaLink, UseCaseCtaRow } from './UseCaseCtaRow.js'
import {
  UseCaseFeatureGrid,
  type UseCaseFeature,
} from './UseCaseFeatureGrid.js'
import { UseCaseSection } from './UseCaseSection.js'

export const metadata = {
  title: 'Spacewave Chat - Encrypted messaging that belongs to you.',
  description:
    'End-to-end encrypted messaging with group channels, no metadata collection, and full history on your devices. Matrix interop included.',
  canonicalPath: '/landing/chat',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

const FEATURES: UseCaseFeature[] = [
  {
    icon: LuLock,
    title: 'End-to-end encrypted',
    description:
      'Every message is encrypted on your device before it leaves. Group chats, direct messages, media. All of it.',
  },
  {
    icon: LuUsers,
    title: 'Group channels',
    description:
      'Create channels for your team, family, or community. Organize conversations by topic. Everyone stays in sync.',
  },
  {
    icon: LuShield,
    title: 'No metadata collection',
    description:
      'Spacewave does not track who you talk to, when you talk, or how often. Your social graph is yours alone.',
  },
  {
    icon: LuSmartphone,
    title: 'History on your devices',
    description:
      'Chat history lives on your hardware, not on a server. Search your full archive offline. Export anytime.',
  },
  {
    icon: LuGlobe,
    title: 'Matrix interop',
    description:
      'Bridge to the Matrix protocol for federation with the wider ecosystem. Talk to anyone on Matrix without leaving Spacewave.',
  },
  {
    icon: LuMessageSquare,
    title: 'Rich messaging',
    description:
      'Markdown formatting, file attachments, link previews. The features you expect from a modern messenger, without the surveillance.',
  },
]

// LandingChat renders the Social & Messaging use-case landing page.
export function LandingChat() {
  const landingHref = useStaticHref('/landing')
  const chatHref = '#/quickstart/chat'

  return (
    <LegalPageLayout
      icon={<LuMessageSquare className="h-8 w-8" />}
      title="Encrypted messaging that belongs to you."
      subtitle="Private conversations for your people. No tracking, no ads, no server-side copies of your messages."
    >
      <UseCaseSection>
        <UseCaseFeatureGrid features={FEATURES} />
      </UseCaseSection>

      <UseCaseSection>
        <UseCaseCallout title="Messaging built on Spacewave">
          <p>
            Spacewave Chat is built on the same encrypted peer-to-peer
            infrastructure as every other Spacewave feature. Messages sync
            through Hydra's content-addressed store and travel over Bifrost's
            encrypted network layer.
          </p>
          <p>
            Your chat history is just data in your Space. Back it up, move it
            between devices, or export it. You own it completely.
          </p>
        </UseCaseCallout>
      </UseCaseSection>

      <UseCaseSection>
        <ChatLandingDemo />
      </UseCaseSection>

      <UseCaseSection>
        <UseCaseCtaRow>
          <UseCaseCtaLink href={chatHref} icon={LuRocket} variant="primary">
            Start a conversation
          </UseCaseCtaLink>
          <UseCaseCtaLink href={landingHref} icon={LuCheck}>
            See all features
          </UseCaseCtaLink>
        </UseCaseCtaRow>
      </UseCaseSection>
    </LegalPageLayout>
  )
}
