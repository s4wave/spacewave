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
  title: 'Spacewave Drive - Private files in your browser.',
  description:
    'Create a private browser file workspace that works offline, syncs through your devices, and uses cloud only when you want backup or reach.',
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
      icon={<LuHardDrive className="size-8" />}
      title="Private files in your browser."
      subtitle="Create a file workspace that works offline, syncs through your devices, and keeps cloud as an optional backup path."
    >
      <UseCaseSection>
        <UseCaseCallout title="The Drive Quickstart creates a real workspace">
          <p>
            Start with folders, files, previews, and first actions in the
            browser. Your laptop can keep working offline, your phone can catch
            up later, and a server or Spacewave Cloud can join only when you
            want backup or reach.
          </p>
          <p>
            The page below is a simulated first look at that workspace. Choose
            Create a Drive when you want the real Quickstart to create the Space
            and open the current Drive surface.
          </p>
        </UseCaseCallout>
      </UseCaseSection>

      <UseCaseSection>
        <UseCaseFeatureGrid features={FEATURES} />
      </UseCaseSection>

      <UseCaseSection>
        <DriveLandingDemo />
      </UseCaseSection>

      <UseCaseSection>
        <UseCaseCallout title="Why the workflow stays yours">
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
