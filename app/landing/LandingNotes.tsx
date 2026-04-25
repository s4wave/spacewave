import {
  LuBookOpen,
  LuCheck,
  LuCloudOff,
  LuFolder,
  LuRocket,
  LuSearch,
  LuWifiOff,
} from 'react-icons/lu'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { NotesLandingDemo } from './LandingDemos.js'
import { LegalPageLayout } from './LegalPageLayout.js'
import { UseCaseCallout } from './UseCaseCallout.js'
import { UseCaseCtaLink, UseCaseCtaRow } from './UseCaseCtaRow.js'
import {
  UseCaseFeatureGrid,
  type UseCaseFeature,
} from './UseCaseFeatureGrid.js'
import { UseCaseSection } from './UseCaseSection.js'

export const metadata = {
  title: 'Spacewave Notes - Think clearly. On your terms.',
  description:
    'Markdown-native notes with offline-first sync, folder organization, and full-text search. No cloud dependency. Your thoughts stay on your devices.',
  canonicalPath: '/landing/notes',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

const FEATURES: UseCaseFeature[] = [
  {
    icon: LuBookOpen,
    title: 'Markdown native',
    description:
      'Write in plain Markdown. No proprietary format, no lock-in. Your notes are portable text files you can open anywhere.',
  },
  {
    icon: LuCloudOff,
    title: 'Offline-first',
    description:
      'Every note lives on your device. Write on a plane, in a cafe, or in the middle of nowhere. Sync happens when you reconnect.',
  },
  {
    icon: LuFolder,
    title: 'Folder organization',
    description:
      'Organize notes into folders and sub-folders. Simple hierarchy that matches how you think. No forced tagging systems.',
  },
  {
    icon: LuSearch,
    title: 'Full-text search',
    description:
      'Find anything across all your notes instantly. Search is local and fast because your data is on your device.',
  },
  {
    icon: LuWifiOff,
    title: 'No cloud dependency',
    description:
      'Spacewave Notes works without any server. Add cloud sync later if you want, but it is never required.',
  },
  {
    icon: LuRocket,
    title: 'Multi-device sync',
    description:
      'Start a note on your laptop, continue on your phone. Changes sync across your swarm in real time via encrypted P2P.',
  },
]

// LandingNotes renders the Knowledge & Planning use-case landing page.
export function LandingNotes() {
  const landingHref = useStaticHref('/landing')
  const notebookHref = '#/quickstart/notebook'

  return (
    <LegalPageLayout
      icon={<LuBookOpen className="h-8 w-8" />}
      title="Think clearly. On your terms."
      subtitle="A place for your thoughts that respects your privacy. Markdown notes that sync across your devices without touching a server."
    >
      <UseCaseSection>
        <UseCaseFeatureGrid features={FEATURES} />
      </UseCaseSection>

      <UseCaseSection>
        <UseCaseCallout title="Notes, built into your Space">
          <p>
            Spacewave Notes stores notebooks in Hydra's content-addressed block
            store, syncs over Bifrost, and renders in the browser via WASM.
          </p>
          <p>
            Your notes are encrypted at rest and in transit. They belong to your
            Space and follow the same backup and sync rules as everything else.
          </p>
        </UseCaseCallout>
      </UseCaseSection>

      <UseCaseSection>
        <NotesLandingDemo />
      </UseCaseSection>

      <UseCaseSection>
        <UseCaseCtaRow>
          <UseCaseCtaLink href={notebookHref} icon={LuRocket} variant="primary">
            Start writing
          </UseCaseCtaLink>
          <UseCaseCtaLink href={landingHref} icon={LuCheck}>
            See all features
          </UseCaseCtaLink>
        </UseCaseCtaRow>
      </UseCaseSection>
    </LegalPageLayout>
  )
}
