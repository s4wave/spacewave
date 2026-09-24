import type { ComponentType } from 'react'
import {
  LuBookOpen,
  LuFileText,
  LuFolder,
  LuLayoutGrid,
  LuNotebookPen,
} from 'react-icons/lu'

import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'

import type { LiveDemoAppProps } from './use-case/LiveDemoApp.js'
import { UseCaseDemo } from './use-case/UseCaseDemo.js'
import { UseCasePage } from './use-case/UseCasePage.js'

export const metadata = {
  title: 'Spacewave Notes - Write it down. Keep it yours.',
  description:
    'Try a real Spacewave notebook in your browser. Notes are Markdown files in a private Space that syncs to the devices you link.',
  canonicalPath: '/landing/notes',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

// LandingNotes renders the Notes use-case page. liveApp mounts the demo notebook.
export function LandingNotes({
  liveApp,
}: {
  liveApp?: ComponentType<LiveDemoAppProps>
}) {
  const landingHref = useStaticHref('/landing')

  return (
    <UseCasePage
      icon={<LuBookOpen className="size-8" />}
      title="Write it down. Keep it yours."
      subtitle="Spacewave Notes keeps a Markdown notebook in a private Space on your own devices. It works offline and syncs when your devices meet."
      points={[
        {
          title: 'Notes are files',
          body: "Each note is a Markdown or Org file in the Space's file system. Drive shows the same files, so nothing is locked in a format.",
        },
        {
          title: 'Offline by default',
          body: 'The notebook lives in local storage on each linked device. Write without a connection and the others catch up when they reconnect.',
        },
        {
          title: 'Encrypted end to end',
          body: 'Notes are encrypted on your devices. Spacewave Cloud backup is optional and only stores data encrypted before upload.',
        },
      ]}
      keepTitle="Keep your notebook"
      keepBody="The demo lives in memory and is gone when you leave. The Notebook Quickstart creates the same Space in your browser's storage, where it stays until you delete it."
      primary={{
        href: '#/quickstart/notebook',
        label: 'Create a notebook',
        icon: LuNotebookPen,
      }}
      secondary={{
        href: landingHref,
        label: 'See all features',
        icon: LuLayoutGrid,
      }}
    >
      <UseCaseDemo
        demo="notes"
        liveApp={liveApp}
        label="Notes"
        poster={<NotesPoster />}
        suggestions={[
          'Open welcome in My Notes to read it.',
          'Press New note, give it a name, and write a few lines of Markdown.',
          'Search notes to filter the list, then press Reset to start over.',
        ]}
      />
    </UseCasePage>
  )
}

// NotesPoster previews the Notebook Quickstart: its note list and the welcome
// note.
function NotesPoster() {
  return (
    <div className="flex h-full text-sm">
      <div className="border-foreground/10 flex w-48 shrink-0 flex-col gap-1 border-r p-3">
        <span className="text-foreground-alt text-metadata px-2 pb-1 font-semibold tracking-wide uppercase">
          My Notes
        </span>
        <span className="text-foreground flex items-center gap-2 px-2 py-1">
          <LuFolder className="text-brand size-4" />
          Org
        </span>
        <span className="text-foreground flex items-center gap-2 px-2 py-1 pl-8">
          <LuFileText className="text-foreground-alt size-4" />
          getting-started
        </span>
        <span className="bg-foreground/5 text-foreground flex items-center gap-2 rounded px-2 py-1">
          <LuFileText className="text-foreground-alt size-4" />
          welcome
        </span>
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-3 p-6">
        <h3 className="text-foreground text-xl font-semibold">Welcome</h3>
        <p className="text-foreground-alt">
          Start capturing notes in this Spacewave notebook.
        </p>
      </div>
    </div>
  )
}
