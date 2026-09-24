import type { ComponentType } from 'react'
import { LuFileText, LuFolder, LuHardDrive, LuLayoutGrid } from 'react-icons/lu'

import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { DRIVE_STARTER_GUIDE_NAME } from '@s4wave/app/quickstart/drive-starter-guide.js'

import {
  DRIVE_DEMO_FILES,
  DRIVE_DEMO_FOLDERS,
} from './use-case/seeds/drive-content.js'
import type { LiveDemoAppProps } from './use-case/LiveDemoApp.js'
import { UseCaseDemo } from './use-case/UseCaseDemo.js'
import { UseCasePage } from './use-case/UseCasePage.js'

export const metadata = {
  title: 'Spacewave Drive - Your files, on your devices.',
  description:
    'Try a real Spacewave Drive in your browser. Files live in a private Space that syncs to the devices you link, with encrypted cloud backup only if you want it.',
  canonicalPath: '/landing/drive',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
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
      icon={<LuHardDrive className="size-8" />}
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
    <div className="flex h-full flex-col text-sm">
      <div className="border-foreground/10 text-foreground-alt border-b px-4 py-2 text-xs">
        Drive /
      </div>
      <ul className="flex flex-col">
        {rows.map((row) => (
          <li
            key={row.name}
            className="border-foreground/5 flex items-center gap-3 border-b px-4 py-2.5"
          >
            {row.folder ? (
              <LuFolder className="text-brand size-4" />
            ) : (
              <LuFileText className="text-foreground-alt size-4" />
            )}
            <span className="text-foreground">{row.name}</span>
            <span className="text-foreground-alt ml-auto text-xs">
              {row.detail}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}
