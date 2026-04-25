import {
  LuCheck,
  LuCloudOff,
  LuFolderSync,
  LuHardDrive,
  LuLock,
  LuRefreshCw,
  LuRocket,
  LuServer,
} from 'react-icons/lu'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { DriveLandingDemo } from './LandingDemos.js'
import { LegalPageLayout } from './LegalPageLayout.js'
import { UseCaseCallout } from './UseCaseCallout.js'
import { UseCaseCtaLink, UseCaseCtaRow } from './UseCaseCtaRow.js'
import {
  UseCaseFeatureGrid,
  type UseCaseFeature,
} from './UseCaseFeatureGrid.js'
import { UseCaseSection } from './UseCaseSection.js'

export const metadata = {
  title: 'Spacewave Drive - Your files, your devices, synced.',
  description:
    'Offline-first file sync with end-to-end encryption. No file size limits. Any storage backend. Conflict resolution built in.',
  canonicalPath: '/landing/drive',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

const FEATURES: UseCaseFeature[] = [
  {
    icon: LuCloudOff,
    title: 'Offline-first',
    description:
      'Every file lives on your device. Edit anything without an internet connection. Changes sync the moment you reconnect.',
  },
  {
    icon: LuLock,
    title: 'End-to-end encrypted',
    description:
      'Files are encrypted before they leave your device. Not even the relay servers can read your data.',
  },
  {
    icon: LuRefreshCw,
    title: 'Automatic conflict resolution',
    description:
      'Edit the same file from two devices offline. Spacewave merges changes intelligently when they reconnect.',
  },
  {
    icon: LuServer,
    title: 'Any storage backend',
    description:
      'Store your data on local disk, S3, a Raspberry Pi, or Spacewave Cloud. Mix and match backends across your swarm.',
  },
  {
    icon: LuHardDrive,
    title: 'No file size limits',
    description:
      'Content-addressed block storage means files of any size transfer efficiently. Large media, datasets, archives.',
  },
  {
    icon: LuFolderSync,
    title: 'Real-time sync',
    description:
      'Changes propagate across your swarm instantly. Watch a file update on your phone seconds after saving on your laptop.',
  },
]

// LandingDrive renders the Files & Data use-case landing page.
export function LandingDrive() {
  const landingHref = useStaticHref('/landing')
  const createDriveHref = '#/quickstart/drive'

  return (
    <LegalPageLayout
      icon={<LuHardDrive className="h-8 w-8" />}
      title="Your files, your devices, synced."
      subtitle="Spacewave Drive gives you file sync that works offline, encrypts everything, and runs on any storage you choose."
    >
      <UseCaseSection>
        <UseCaseFeatureGrid features={FEATURES} />
      </UseCaseSection>

      <UseCaseSection>
        <UseCaseCallout title="How Spacewave Drive works">
          <p>
            Spacewave Drive uses a content-addressed block DAG (Hydra) to store
            and sync your files. Each file is split into blocks, encrypted, and
            distributed across your devices.
          </p>
          <p>
            When you edit a file, only the changed blocks propagate. Your
            devices find each other directly via Bifrost and sync over encrypted
            peer-to-peer connections.
          </p>
        </UseCaseCallout>
      </UseCaseSection>

      <UseCaseSection>
        <DriveLandingDemo />
      </UseCaseSection>

      <UseCaseSection>
        <UseCaseCtaRow>
          <UseCaseCtaLink
            href={createDriveHref}
            icon={LuRocket}
            variant="primary"
          >
            Create a Drive
          </UseCaseCtaLink>
          <UseCaseCtaLink href={landingHref} icon={LuCheck}>
            See all features
          </UseCaseCtaLink>
        </UseCaseCtaRow>
      </UseCaseSection>
    </LegalPageLayout>
  )
}
