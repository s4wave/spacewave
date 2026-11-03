import type { ComponentType } from 'react'
import {
  LuFileText,
  LuFolder,
  LuHardDrive,
  LuLayoutGrid,
} from '@s4wave/web/images/icons.js'

import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { DRIVE_STARTER_GUIDE_NAME } from '@s4wave/app/quickstart/drive-starter-guide.js'

import {
  DRIVE_DEMO_FILES,
  DRIVE_DEMO_FOLDERS,
} from './use-case/seeds/drive-content.js'
import type { LiveDemoAppProps } from './use-case/LiveDemoApp.js'
import { UseCaseDemo } from './use-case/UseCaseDemo.js'
import { UseCasePage } from './use-case/UseCasePage.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    height: '100%',
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule3: { display: 'flex', flexDirection: 'column' },
  rule4: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule5: { color: themeColors['--color-foreground'] },
  rule6: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginLeft: 'auto',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule2: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

export const metadata = {
  title: 'Spacewave Drive - Your files, on your devices.',
  description:
    'Try a real Spacewave Drive in your browser. Files live in a private Space that syncs to the devices you link, with encrypted cloud backup only if you want it.',
  canonicalPath: '/landing/drive',
}

// LandingDrive renders the Drive use-case page. liveApp mounts the demo Drive.
export function LandingDrive({
  liveApp,
}: {
  liveApp?: ComponentType<LiveDemoAppProps>
}) {
  const landingHref = useStaticHref('/landing')

  return (
    <UseCasePage
      icon={<LuHardDrive sx={stylexStyles.rule1} />}
      title="Your files, on your devices."
      subtitle="Spacewave Drive keeps a private file space in your browser, syncs it to the devices you link, and backs it up to the cloud only if you ask."
      points={[
        {
          title: 'A Space holds your files',
          body: 'Drive is one object in a Space. The same Space can also hold notes, chats and more, and they sync together.',
        },
        {
          title: 'Every device keeps a copy',
          body: 'Link a laptop or phone and the Space syncs to it. Each device keeps its own copy in local storage, so files open without a connection.',
        },
        {
          title: 'Encrypted end to end',
          body: 'Data is encrypted on your devices. Spacewave Cloud backup is optional and only stores data encrypted before upload.',
        },
      ]}
      keepTitle="Keep your files"
      keepBody="The demo lives in memory and is gone when you leave. The Drive Quickstart creates the same Space in your browser's storage, where it stays until you delete it."
      primary={{
        href: '#/quickstart/drive',
        label: 'Create a Drive',
        icon: LuHardDrive,
      }}
      secondary={{
        href: landingHref,
        label: 'See all features',
        icon: LuLayoutGrid,
      }}
    >
      <UseCaseDemo
        demo="drive"
        liveApp={liveApp}
        label="Drive"
        poster={<DrivePoster />}
        suggestions={[
          `Open ${DRIVE_STARTER_GUIDE_NAME} or Photos/harbor.svg to preview it.`,
          'Drag a file from your computer onto the list to upload it.',
          'Make a folder, then press Reset to start over.',
        ]}
      />
    </UseCasePage>
  )
}

// DrivePoster previews the demo Drive's root folder.
function DrivePoster() {
  const rows = [
    ...DRIVE_DEMO_FOLDERS.map((name) => ({
      name,
      folder: true,
      detail: `${DRIVE_DEMO_FILES.filter((file) => file.path.startsWith(name + '/')).length} items`,
    })),
    { name: DRIVE_STARTER_GUIDE_NAME, folder: false, detail: 'Markdown' },
  ]

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>Drive /</div>
      <ul sx={stylexStyles2.rule3}>
        {rows.map((row) => (
          <li key={row.name} sx={stylexStyles2.rule4}>
            {row.folder ? (
              <LuFolder sx={stylexStyles.rule2} />
            ) : (
              <LuFileText sx={stylexStyles.rule3} />
            )}
            <span sx={stylexStyles2.rule5}>{row.name}</span>
            <span sx={stylexStyles2.rule6}>{row.detail}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}
