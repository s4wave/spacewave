import type { ComponentType } from 'react'
import {
  LuBookOpen,
  LuFileText,
  LuFolder,
  LuLayoutGrid,
  LuNotebookPen,
} from '@s4wave/web/images/icons.js'

import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'

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
    height: '100%',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule2: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 48)`,
    flexShrink: 0,
    flexDirection: 'column',
    gap: themeLayout['--spacing'],
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBottom: themeLayout['--spacing'],
    fontSize: themeLayout['--text-metadata'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    color: themeColors['--color-foreground'],
  },
  rule5: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    paddingLeft: `calc(${themeLayout['--spacing']} * 8)`,
    color: themeColors['--color-foreground'],
  },
  rule6: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    color: themeColors['--color-foreground'],
  },
  rule7: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule8: {
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule9: { color: themeColors['--color-foreground-alt'] },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
  },
})

export const metadata = {
  title: 'Spacewave Notes - Write it down. Keep it yours.',
  description:
    'Try a real Spacewave notebook in your browser. Notes are Markdown files in a private Space that syncs to the devices you link.',
  canonicalPath: '/landing/notes',
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
      icon={<LuBookOpen sx={stylexStyles.rule1} />}
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
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <span sx={stylexStyles2.rule3}>My Notes</span>
        <span sx={stylexStyles2.rule4}>
          <LuFolder sx={stylexStyles.rule2} />
          Org
        </span>
        <span sx={stylexStyles2.rule5}>
          <LuFileText sx={stylexStyles.rule3} />
          getting-started
        </span>
        <span sx={stylexStyles2.rule6}>
          <LuFileText sx={stylexStyles.rule3} />
          welcome
        </span>
      </div>
      <div sx={stylexStyles2.rule7}>
        <h3 sx={stylexStyles2.rule8}>Welcome</h3>
        <p sx={stylexStyles2.rule9}>
          Start capturing notes in this Spacewave notebook.
        </p>
      </div>
    </div>
  )
}
