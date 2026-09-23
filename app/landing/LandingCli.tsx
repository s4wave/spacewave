import { LuDownload, LuLayoutGrid, LuTerminal } from 'react-icons/lu'

import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { cn } from '@s4wave/web/style/utils.js'

import { UseCasePage } from './use-case/UseCasePage.js'

export const metadata = {
  title: 'Spacewave CLI - Your Spaces from the terminal.',
  description:
    'The spacewave command runs the full Spacewave runtime in your terminal. Create Spaces, write files and link devices from a shell or script.',
  canonicalPath: '/landing/cli',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

// CliStep is one group of commands in the CLI walkthrough.
interface CliStep {
  title: string
  lines: string[]
}

// CLI_STEPS walks from a new account to files, devices and the web app, using
// only commands the spacewave binary provides.
const CLI_STEPS: CliStep[] = [
  {
    title: 'Start a local account',
    lines: [
      '# Create an offline account on this machine',
      'spacewave login local',
      '# Or add this machine to an existing account',
      'spacewave login pair',
    ],
  },
  {
    title: 'Work with files',
    lines: [
      'spacewave space create "My Space"',
      'spacewave space object create --type fs files',
      'echo hello | spacewave fs write files/-/greeting.txt',
      'spacewave fs cat files/-/greeting.txt',
    ],
  },
  {
    title: 'Link a computer',
    lines: [
      '# On the new computer: print a setup ticket',
      'spacewave device setup',
      '# On a linked computer: approve it',
      'spacewave device approve <ticket>',
    ],
  },
  {
    title: 'Open the app',
    lines: ['# Serve the Spacewave web app on localhost', 'spacewave web'],
  },
]

// LandingCli renders the CLI use-case page: a walkthrough of real commands.
export function LandingCli() {
  const landingHref = useStaticHref('/landing')
  const downloadHref = useStaticHref('/download/cli')

  return (
    <UseCasePage
      icon={<LuTerminal className="size-8" />}
      title="Your Spaces from the terminal."
      subtitle="The spacewave command runs the same runtime as the app. It starts its daemon on first use, so every command works from a shell, a script or a headless server."
      points={[
        {
          title: 'One binary',
          body: 'The CLI is a single spacewave binary for Linux, macOS and Windows. It needs no browser and no other services.',
        },
        {
          title: 'Same Spaces as the app',
          body: 'A Space created in the terminal is the same Space the app opens, and files written with fs write show up there.',
        },
        {
          title: 'Built for scripts',
          body: 'Commands read stdin and write stdout, and status commands accept --output json for other tools to parse.',
        },
      ]}
      keepTitle="Install the CLI"
      keepBody="Download the spacewave binary for your platform, put it on your PATH and run spacewave status to confirm it works."
      primary={{
        href: downloadHref,
        label: 'Download the CLI',
        icon: LuDownload,
      }}
      secondary={{
        href: landingHref,
        label: 'See all features',
        icon: LuLayoutGrid,
      }}
    >
      <section className="relative z-10 mx-auto grid w-full max-w-6xl gap-4 px-4 @lg:grid-cols-2 @lg:px-8">
        {CLI_STEPS.map((step) => (
          <div
            key={step.title}
            className="border-foreground/10 bg-background flex flex-col overflow-hidden rounded-xl border"
          >
            <h2 className="border-foreground/10 text-foreground border-b px-4 py-2.5 text-sm font-semibold">
              {step.title}
            </h2>
            <pre className="overflow-x-auto p-4 font-mono text-xs leading-relaxed">
              {step.lines.map((line) => (
                <div
                  key={line}
                  className={cn(
                    'text-foreground',
                    line.startsWith('#') && 'text-foreground-alt',
                  )}
                >
                  {line}
                </div>
              ))}
            </pre>
          </div>
        ))}
      </section>
    </UseCasePage>
  )
}
