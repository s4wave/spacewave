import {
  LuCheck,
  LuCode,
  LuCpu,
  LuGlobe,
  LuMonitor,
  LuRocket,
  LuTerminal,
} from 'react-icons/lu'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { LegalPageLayout } from './LegalPageLayout.js'
import { UseCaseCallout } from './UseCaseCallout.js'
import { UseCaseCtaLink, UseCaseCtaRow } from './UseCaseCtaRow.js'
import {
  UseCaseFeatureGrid,
  type UseCaseFeature,
} from './UseCaseFeatureGrid.js'
import { UseCaseSection } from './UseCaseSection.js'

export const metadata = {
  title: 'Spacewave CLI - Your swarm from the command line.',
  description:
    'Full feature parity with the GUI. Scriptable, pipe-friendly output, headless server support, and a WASM build for the browser.',
  canonicalPath: '/landing/cli',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

const FEATURES: UseCaseFeature[] = [
  {
    icon: LuTerminal,
    title: 'Full feature parity',
    description:
      'Everything you can do in the GUI, you can do from the terminal. Create Spaces, manage devices, sync files, send messages.',
  },
  {
    icon: LuCode,
    title: 'Scriptable',
    description:
      'Automate your workflow with shell scripts. The CLI is designed for composition with other Unix tools.',
  },
  {
    icon: LuMonitor,
    title: 'Pipe-friendly output',
    description:
      'JSON and plain text output modes. Pipe CLI results into jq, grep, awk, or any tool in your chain.',
  },
  {
    icon: LuCpu,
    title: 'Headless servers',
    description:
      'Run Spacewave on servers without a display. The CLI is the primary interface for headless deployments and background daemons.',
  },
  {
    icon: LuGlobe,
    title: 'WASM build for browser',
    description:
      'The same CLI binary compiles to WebAssembly. Run Spacewave commands directly in the browser terminal.',
  },
  {
    icon: LuRocket,
    title: 'Cross-platform',
    description:
      'Native binaries for Linux, macOS, Windows, and ARM. Single static binary, no dependencies, no installation steps.',
  },
]

const TERMINAL_LINES = [
  { prompt: true, text: 'spacewave space create "my-project"' },
  { prompt: false, text: 'Space created: my-project (local)' },
  { prompt: false, text: '' },
  { prompt: true, text: 'spacewave device link --qr' },
  { prompt: false, text: 'Scan QR code on your phone to join the swarm:' },
  { prompt: false, text: '[QR code displayed]' },
  { prompt: false, text: '' },
  { prompt: true, text: 'spacewave drive sync ./docs' },
  { prompt: false, text: 'Syncing 142 files to "my-project/docs"...' },
  { prompt: false, text: '142/142 files synced (3.2 MB)' },
  { prompt: false, text: '' },
  { prompt: true, text: 'spacewave device list --json | jq ".[].name"' },
  { prompt: false, text: '"laptop"' },
  { prompt: false, text: '"phone"' },
  { prompt: false, text: '"pi-server"' },
]

// LandingCli renders the CLI use-case landing page.
export function LandingCli() {
  const landingHref = useStaticHref('/landing')
  const downloadCliHref = useStaticHref('/download#cli')

  return (
    <LegalPageLayout
      icon={<LuTerminal className="h-8 w-8" />}
      title="Your swarm from the command line."
      subtitle="A terminal-first interface for your entire Spacewave system. Scriptable, composable, and built for automation."
    >
      <UseCaseSection>
        <UseCaseFeatureGrid features={FEATURES} />
      </UseCaseSection>

      <UseCaseSection>
        <UseCaseCallout title="One binary, every platform">
          <p>
            The Spacewave CLI is a single static binary. Download it, run it. No
            package managers, no runtime dependencies, no setup wizards.
          </p>
          <p>
            The same Go codebase compiles to native binaries for all major
            platforms and to WebAssembly for the browser. Your scripts work
            everywhere your code does.
          </p>
        </UseCaseCallout>
      </UseCaseSection>

      <UseCaseSection>
        <div className="border-foreground/10 bg-background/60 overflow-hidden rounded-lg border backdrop-blur-sm">
          <div className="border-foreground/8 flex items-center gap-2 border-b px-4 py-2.5">
            <div className="bg-destructive/60 h-3 w-3 rounded-full" />
            <div className="bg-warning/60 h-3 w-3 rounded-full" />
            <div className="bg-success/60 h-3 w-3 rounded-full" />
            <span className="text-foreground-alt ml-2 font-mono text-xs">
              spacewave
            </span>
          </div>
          <div className="p-4 font-mono text-sm leading-relaxed">
            {TERMINAL_LINES.map((line, i) => (
              <div key={i} className="whitespace-pre">
                {line.text === '' ?
                  '\u00A0'
                : line.prompt ?
                  <>
                    <span className="text-brand">$</span>{' '}
                    <span className="text-foreground">{line.text}</span>
                  </>
                : <span className="text-foreground-alt">{line.text}</span>}
              </div>
            ))}
          </div>
        </div>
      </UseCaseSection>

      <UseCaseSection>
        <UseCaseCtaRow>
          <UseCaseCtaLink
            href={downloadCliHref}
            icon={LuRocket}
            variant="primary"
          >
            Download the CLI
          </UseCaseCtaLink>
          <UseCaseCtaLink href={landingHref} icon={LuCheck}>
            See all features
          </UseCaseCtaLink>
        </UseCaseCtaRow>
      </UseCaseSection>
    </LegalPageLayout>
  )
}
