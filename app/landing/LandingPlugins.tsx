import {
  LuBookOpen,
  LuCheck,
  LuCode,
  LuCpu,
  LuGlobe,
  LuPuzzle,
  LuRefreshCw,
} from 'react-icons/lu'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { PluginsLandingDemo } from './LandingDemos.js'
import { LegalPageLayout } from './LegalPageLayout.js'
import { UseCaseCallout } from './UseCaseCallout.js'
import { UseCaseCtaLink, UseCaseCtaRow } from './UseCaseCtaRow.js'
import {
  UseCaseFeatureGrid,
  type UseCaseFeature,
} from './UseCaseFeatureGrid.js'
import { UseCaseSection } from './UseCaseSection.js'

export const metadata = {
  title: 'Spacewave Plugins - Build anything. Ship it everywhere.',
  description:
    'Full-stack SDK for Go and TypeScript. WASM runtime, hot reload, proto-first APIs. Build plugins that run in browsers, on desktops, and on embedded devices.',
  canonicalPath: '/landing/plugins',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

const FEATURES: UseCaseFeature[] = [
  {
    icon: LuCode,
    title: 'Go + TypeScript',
    description:
      'Write your plugin in Go, TypeScript, or both. The SDK covers databases, networking, files, and UI in a single API.',
  },
  {
    icon: LuCpu,
    title: 'WebAssembly runtime',
    description:
      'Go plugins compile to WASM and run in the browser alongside the app. Same binary runs on desktop, mobile, and embedded targets.',
  },
  {
    icon: LuRefreshCw,
    title: 'Hot reload',
    description:
      'Change your code and see it update instantly. The ControllerBus hot-loads new plugin configurations without restarting.',
  },
  {
    icon: LuPuzzle,
    title: 'Proto-first APIs',
    description:
      'Define your plugin interface in protobuf. Get type-safe RPC clients in Go and TypeScript generated automatically.',
  },
  {
    icon: LuGlobe,
    title: 'Browser + desktop + embedded',
    description:
      'One codebase. Ship to Chrome, Electron, Raspberry Pi, or a headless server. Cross-compile with the same toolchain.',
  },
  {
    icon: LuBookOpen,
    title: 'Full documentation',
    description:
      'The same SDK that built Spacewave. Every API is documented, every pattern has examples. Start building in minutes.',
  },
]

// LandingPlugins renders the Apps & Tools use-case landing page.
export function LandingPlugins() {
  const landingHref = useStaticHref('/landing')
  const docsHref = '#/docs'

  return (
    <LegalPageLayout
      icon={<LuPuzzle className="h-8 w-8" />}
      title="Build anything. Ship it everywhere."
      subtitle="The Spacewave SDK gives you databases, networking, encryption, and UI in one package. Write once, deploy to any platform."
    >
      <UseCaseSection>
        <UseCaseFeatureGrid features={FEATURES} />
      </UseCaseSection>

      <UseCaseSection>
        <UseCaseCallout title="The Spacewave stack">
          <p>
            Plugins run on top of four layers: Bifrost (encrypted networking),
            Hydra (data storage), ControllerBus (lifecycle coordination), and
            the Spacewave SDK (application framework).
          </p>
          <p>
            Each layer is open-source and usable independently. Together they
            give you a complete platform for building distributed applications.
          </p>
        </UseCaseCallout>
      </UseCaseSection>

      <UseCaseSection>
        <PluginsLandingDemo />
      </UseCaseSection>

      <UseCaseSection>
        <UseCaseCtaRow>
          <UseCaseCtaLink href={docsHref} icon={LuBookOpen} variant="primary">
            Read the SDK docs
          </UseCaseCtaLink>
          <UseCaseCtaLink href={landingHref} icon={LuCheck}>
            See all features
          </UseCaseCtaLink>
        </UseCaseCtaRow>
      </UseCaseSection>
    </LegalPageLayout>
  )
}
