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
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    height: '100%',
  },
  rule2: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    gap: themeLayout['--spacing'],
    display: 'flex',
    flexDirection: 'column',
    flexShrink: 0,
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    width: `calc(${themeLayout['--spacing']} * 48)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeLayout['--text-metadata'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    textTransform: 'uppercase',
    paddingBottom: themeLayout['--spacing'],
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    paddingLeft: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule6: {
    borderRadius: '.25rem',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground'],
    display: 'flex',
  },
  rule7: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexDirection: 'column',
    minWidth: 0,
  },
  rule8: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
  },
  rule9: { color: themeColors['--color-foreground-alt'] },
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
