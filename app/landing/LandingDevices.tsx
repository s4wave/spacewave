import type { ComponentType } from 'react'
import {
  LuHardDrive,
  LuLayoutGrid,
  LuMonitor,
  LuPlus,
  LuServer,
} from 'react-icons/lu'

import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'

import type { LiveDemoAppProps } from './use-case/LiveDemoApp.js'
import { UseCaseDemo } from './use-case/UseCaseDemo.js'
import { UseCasePage } from './use-case/UseCasePage.js'

export const metadata = {
  title: 'Spacewave Devices - Your computers, in one Space.',
  description:
    'Try the Spacewave Computers dashboard in your browser. Link a machine with the Spacewave CLI, or add an SSH host and open a terminal to it.',
  canonicalPath: '/landing/devices',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

// LandingDevices renders the Devices use-case page. liveApp mounts the demo
// Computers dashboard.
export function LandingDevices({
  liveApp,
}: {
  liveApp?: ComponentType<LiveDemoAppProps>
}) {
  const landingHref = useStaticHref('/landing')

  return (
    <UseCasePage
      icon={<LuMonitor className="size-8" />}
      title="Your computers, in one Space."
      subtitle="Spacewave Devices lists the machines you work with in a private Space: Devices you link with the Spacewave CLI, and SSH hosts you connect to on demand."
      points={[
        {
          title: 'Link with a ticket',
          body: 'Run spacewave device setup on a computer, paste the ticket it prints into Add Device, and approve it. The computer joins the Space as a managed Device.',
        },
        {
          title: 'SSH hosts, no agent',
          body: 'Add an existing machine as an SSH Host and open a terminal to it over SSH, without installing an agent on it.',
        },
        {
          title: 'One inventory',
          body: 'Devices and hosts are objects in the Space, so every device linked to it sees the same list.',
        },
      ]}
      keepTitle="Keep your dashboard"
      keepBody="The demo lives in memory and is gone when you leave. The Add a Device Quickstart creates the same dashboard in your browser's storage, ready for your first computer."
      primary={{
        href: '#/quickstart/device',
        label: 'Add a device',
        icon: LuMonitor,
      }}
      secondary={{
        href: landingHref,
        label: 'See all features',
        icon: LuLayoutGrid,
      }}
    >
      <UseCaseDemo
        demo="devices"
        liveApp={liveApp}
        label="Devices"
        poster={<DevicesPoster />}
        suggestions={[
          'Press Add Device and compare a Managed Device with an SSH Host.',
          'Choose SSH Host to see what an on-demand terminal needs.',
          'Press Reset to return to the empty dashboard.',
        ]}
      />
    </UseCasePage>
  )
}

// DevicesPoster previews the Device Quickstart: an empty Computers dashboard.
function DevicesPoster() {
  const tiles = [
    { label: 'Managed Devices', icon: LuHardDrive },
    { label: 'SSH Hosts', icon: LuServer },
  ]

  return (
    <div className="flex h-full flex-col text-sm">
      <div className="border-foreground/10 flex items-center justify-between border-b px-4 py-2">
        <span className="text-foreground flex items-center gap-2 font-semibold">
          <LuMonitor className="size-4" />
          Computers
        </span>
        <span className="border-foreground/10 text-foreground flex items-center gap-1.5 rounded border px-2 py-1 text-xs">
          <LuPlus className="size-3.5" />
          Add Device
        </span>
      </div>
      <div className="flex flex-col gap-4 p-4">
        <div className="grid grid-cols-2 gap-3">
          {tiles.map((tile) => (
            <div
              key={tile.label}
              className="border-foreground/10 rounded-lg border p-3.5"
            >
              <span className="text-foreground-alt flex items-center gap-1.5 text-xs font-medium">
                <tile.icon className="size-3.5" />
                {tile.label}
              </span>
              <div className="text-foreground mt-2 text-xl font-semibold">
                0
              </div>
            </div>
          ))}
        </div>
        <div className="border-foreground/10 rounded-lg border p-3.5">
          <span className="text-foreground text-xs font-medium">
            No computers added
          </span>
          <p className="text-foreground-alt mt-1 text-xs leading-relaxed">
            Add a managed Device for persistent capabilities, or add an SSH Host
            for on-demand terminal access.
          </p>
        </div>
      </div>
    </div>
  )
}
