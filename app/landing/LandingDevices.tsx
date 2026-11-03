import type { ComponentType } from 'react'
import {
  LuHardDrive,
  LuLayoutGrid,
  LuMonitor,
  LuPlus,
  LuServer,
} from '@s4wave/web/images/icons.js'

import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'

import type { LiveDemoAppProps } from './use-case/LiveDemoApp.js'
import { UseCaseDemo } from './use-case/UseCaseDemo.js'
import { UseCasePage } from './use-case/UseCasePage.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  utilityState as themeUtilityState,
  colors as themeColors,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    flexDirection: 'column',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule5: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule6: {
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule7: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule8: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
  },
  rule9: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule10: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule11: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export const metadata = {
  title: 'Spacewave Devices - Your computers, in one Space.',
  description:
    'Try the Spacewave Computers dashboard in your browser. Link a machine with the Spacewave CLI, or add an SSH host and open a terminal to it.',
  canonicalPath: '/landing/devices',
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
      icon={<LuMonitor sx={stylexStyles.rule1} />}
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
    <div sx={stylexStyles3.rule1}>
      <div sx={stylexStyles3.rule2}>
        <span sx={stylexStyles3.rule3}>
          <LuMonitor sx={stylexStyles.rule2} />
          Computers
        </span>
        <span sx={stylexStyles3.rule4}>
          <LuPlus sx={stylexStyles.rule3} />
          Add Device
        </span>
      </div>
      <div sx={stylexStyles3.rule5}>
        <div sx={stylexStyles3.rule6}>
          {tiles.map((tile) => (
            <div key={tile.label} sx={stylexStyles3.rule7}>
              <span sx={stylexStyles3.rule8}>
                <tile.icon sx={stylexStyles2.rule1} />
                {tile.label}
              </span>
              <div sx={stylexStyles3.rule9}>0</div>
            </div>
          ))}
        </div>
        <div sx={stylexStyles3.rule7}>
          <span sx={stylexStyles3.rule10}>No computers added</span>
          <p sx={stylexStyles3.rule11}>
            Add a managed Device for persistent capabilities, or add an SSH Host
            for on-demand terminal access.
          </p>
        </div>
      </div>
    </div>
  )
}
