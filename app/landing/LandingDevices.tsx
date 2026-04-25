import {
  LuCheck,
  LuGlobe,
  LuHeartPulse,
  LuLock,
  LuRocket,
  LuServer,
  LuTerminal,
} from 'react-icons/lu'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { DevicesLandingDemo } from './LandingDemos.js'
import { LegalPageLayout } from './LegalPageLayout.js'
import { UseCaseCallout } from './UseCaseCallout.js'
import { UseCaseCtaLink, UseCaseCtaRow } from './UseCaseCtaRow.js'
import {
  UseCaseFeatureGrid,
  type UseCaseFeature,
} from './UseCaseFeatureGrid.js'
import { UseCaseSection } from './UseCaseSection.js'

export const metadata = {
  title: 'Spacewave Devices - Every device in your swarm. One command away.',
  description:
    'Remote terminal, direct P2P connections, works behind NAT with no port forwarding. Device health monitoring across your whole swarm.',
  canonicalPath: '/landing/devices',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

const FEATURES: UseCaseFeature[] = [
  {
    icon: LuTerminal,
    title: 'Remote terminal',
    description:
      'Open a terminal on any device in your swarm from your browser or desktop. Full PTY support, no SSH configuration required.',
  },
  {
    icon: LuLock,
    title: 'Peer-to-peer direct',
    description:
      'Devices connect directly to each other over encrypted channels. No traffic routes through third-party servers.',
  },
  {
    icon: LuGlobe,
    title: 'Works behind NAT',
    description:
      'Bifrost handles NAT traversal automatically. Connect to devices on home networks, behind firewalls, or on cellular data.',
  },
  {
    icon: LuServer,
    title: 'No port forwarding',
    description:
      'Forget about configuring routers. Spacewave punches through NAT and firewalls without exposing ports to the internet.',
  },
  {
    icon: LuHeartPulse,
    title: 'Device health monitoring',
    description:
      'See which devices are online, their connection quality, and resource usage. Know the state of your swarm at a glance.',
  },
  {
    icon: LuRocket,
    title: 'Zero configuration',
    description:
      'Add a device by scanning a QR code or pasting a link. No DNS, no certificates, no firewall rules. It just works.',
  },
]

// LandingDevices renders the Devices & Servers use-case landing page.
export function LandingDevices() {
  const landingHref = useStaticHref('/landing')
  const linkDeviceHref = '#/pair'

  return (
    <LegalPageLayout
      icon={<LuServer className="h-8 w-8" />}
      title="Every device in your swarm. One command away."
      subtitle="Manage laptops, phones, Raspberry Pis, and servers from anywhere. Direct encrypted connections with zero configuration."
    >
      <UseCaseSection>
        <UseCaseFeatureGrid features={FEATURES} />
      </UseCaseSection>

      <UseCaseSection>
        <UseCaseCallout title="Your devices form a mesh">
          <p>
            Each device in your Spacewave swarm is a first-class node. Laptops,
            phones, single-board computers, cloud VMs. They discover each other
            automatically and maintain encrypted tunnels.
          </p>
          <p>
            Add a headless server with the CLI. Add a phone with a QR code.
            Every device gets the same capabilities: file access, terminal, data
            sync, and plugin execution.
          </p>
        </UseCaseCallout>
      </UseCaseSection>

      <UseCaseSection>
        <DevicesLandingDemo />
      </UseCaseSection>

      <UseCaseSection>
        <UseCaseCtaRow>
          <UseCaseCtaLink
            href={linkDeviceHref}
            icon={LuRocket}
            variant="primary"
          >
            Link a device
          </UseCaseCtaLink>
          <UseCaseCtaLink href={landingHref} icon={LuCheck}>
            See all features
          </UseCaseCtaLink>
        </UseCaseCtaRow>
      </UseCaseSection>
    </LegalPageLayout>
  )
}
