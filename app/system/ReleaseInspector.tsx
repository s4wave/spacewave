import type { AppBuildInfo } from '@s4wave/app/build-info.js'
import type { RecoveryStatus } from '@s4wave/sdk/status/status.pb.js'

import { Facts } from './Facts.js'
import { InspectorSection } from './InspectorSection.js'
import { updatePhaseLabel } from './useSystemModel.js'

type Launcher = NonNullable<RecoveryStatus['launcher']>
type NativePackage = NonNullable<RecoveryStatus['nativePackages']>[number]

/** BuildSection renders the facts of this app build. */
function BuildSection({ build }: { build: AppBuildInfo }) {
  return (
    <InspectorSection title="This build">
      <Facts
        facts={[
          { label: 'Version', value: build.version || 'dev', mono: true },
          { label: 'Main version', value: build.mainVersion, mono: true },
          { label: 'Runtime', value: build.runtimeLabel },
          {
            label: 'Platform',
            value:
              build.goos && build.goarch
                ? `${build.goos}/${build.goarch}`
                : null,
            mono: true,
          },
          { label: 'Go', value: build.goVersion, mono: true },
          {
            label: 'Browser generation',
            value: build.browserGenerationId,
            mono: true,
          },
        ]}
      />
    </InspectorSection>
  )
}

/** AppUpdateSection renders the launcher's staged desktop app update. */
function AppUpdateSection({
  recovery,
  launcher,
}: {
  recovery: RecoveryStatus | null
  launcher: Launcher | undefined
}) {
  return (
    <InspectorSection
      title="Installed app update"
      description="The launcher stages the signed desktop app. Installing it restarts this app while the shared daemon keeps running."
    >
      <Facts
        empty={
          recovery
            ? 'This runtime has no launcher.'
            : 'Waiting for the recovery watcher.'
        }
        facts={
          launcher
            ? [
                {
                  label: 'App state',
                  value: updatePhaseLabel(launcher.updatePhase),
                  tone: launcher.updatePhase === 'error' ? 'error' : undefined,
                },
                {
                  label: 'App version',
                  value: launcher.updateVersion,
                  mono: true,
                },
                {
                  label: 'Error',
                  value: launcher.updateError,
                  tone: 'error',
                },
                { label: 'Channel', value: launcher.selectedChannelKey },
                {
                  label: 'Staged app',
                  value: launcher.stagedPath,
                  mono: true,
                },
                {
                  label: 'Release metadata',
                  value: launcher.releaseMetadataOutcome,
                },
              ]
            : []
        }
      />
    </InspectorSection>
  )
}

/** DaemonUpdateSection renders the staged shared daemon update. */
function DaemonUpdateSection({ launcher }: { launcher: Launcher | undefined }) {
  return (
    <InspectorSection
      title="Shared daemon update"
      description="The CLI artifact is staged separately; daemon replacement waits for its own idle update operation."
    >
      <Facts
        empty={
          launcher
            ? 'No daemon artifact selected.'
            : 'Waiting for the recovery watcher.'
        }
        facts={
          launcher
            ? [
                {
                  label: 'Daemon state',
                  value: updatePhaseLabel(launcher.daemonUpdatePhase),
                },
                {
                  label: 'Daemon version',
                  value: launcher.daemonUpdateVersion,
                  mono: true,
                },
                {
                  label: 'CLI artifact',
                  value: launcher.selectedCliManifestId,
                  mono: true,
                },
                {
                  label: 'Staged daemon',
                  value:
                    launcher.daemonStagedPath ?? launcher.selectedCliBinaryPath,
                  mono: true,
                },
              ]
            : []
        }
      />
    </InspectorSection>
  )
}

/** LauncherConfigSection renders the selected and fetched launcher configuration. */
function LauncherConfigSection({
  launcher,
}: {
  launcher: Launcher | undefined
}) {
  return (
    <InspectorSection
      title="Launcher config"
      description="The configuration revision in use and the newest one fetched."
    >
      <Facts
        facts={[
          {
            label: 'Selected revision',
            value: revisionFact(
              launcher?.selectedConfigRev,
              launcher?.selectedConfigSource,
            ),
            mono: true,
          },
          {
            label: 'Fetched revision',
            value: revisionFact(
              launcher?.fetchedConfigRev,
              launcher?.fetchedConfigSource,
            ),
            mono: true,
          },
          {
            label: 'Release world head',
            value: launcher?.releaseWorldHeadRef,
            mono: true,
          },
          {
            label: 'Entrypoint manifest',
            value: launcher?.selectedEntrypointManifestId,
            mono: true,
          },
          {
            label: 'Entrypoint platform',
            value: launcher?.selectedEntrypointPlatformId,
            mono: true,
          },
          {
            label: 'Manifest revision',
            value: launcher?.selectedEntrypointManifestRev
              ? String(launcher.selectedEntrypointManifestRev)
              : null,
            mono: true,
          },
          {
            label: 'Manifest ref',
            value: launcher?.selectedEntrypointManifestRef,
            mono: true,
          },
        ]}
      />
    </InspectorSection>
  )
}

/** BootSection renders what the browser reported at app start. */
function BootSection({ boot }: { boot: RecoveryStatus['boot'] }) {
  return (
    <InspectorSection
      title="Boot"
      description="What the browser reported when this app started."
    >
      <Facts
        empty="No boot report from this runtime."
        facts={
          boot?.status === 'reported'
            ? [
                {
                  label: 'Compatibility',
                  value: boot.compatibilityVersion,
                  mono: true,
                },
                { label: 'Last reset', value: boot.lastResetDecision },
              ]
            : []
        }
      />
    </InspectorSection>
  )
}

/** RuntimeAssetSection renders how the runtime entry script was fetched and loaded. */
function RuntimeAssetSection({
  asset,
}: {
  asset: RecoveryStatus['runtimeAsset']
}) {
  return (
    <InspectorSection
      title="Runtime asset"
      description="How the runtime entry script was fetched and loaded."
    >
      <Facts
        empty="No runtime asset report from this runtime."
        facts={
          asset?.status === 'reported'
            ? [
                {
                  label: 'Result',
                  value: asset.ok ? 'Loaded' : 'Failed',
                  tone: asset.ok ? undefined : 'error',
                },
                { label: 'HTTP status', value: asset.statusCode, mono: true },
                { label: 'Classification', value: asset.classification },
                { label: 'Fetched from', value: asset.fetchSource },
                { label: 'Script', value: asset.scriptPath, mono: true },
                {
                  label: 'Content type',
                  value: asset.contentType,
                  mono: true,
                },
                {
                  label: 'Plugin asset',
                  value: asset.pluginAssetResult,
                },
                {
                  label: 'Runtime error',
                  value: asset.runtimeError,
                  tone: 'error',
                },
                { label: 'Body prefix', value: asset.bodyPrefix, mono: true },
              ]
            : []
        }
      />
    </InspectorSection>
  )
}

/** NativePackagesSection lists the platform binaries unpacked for plugins. */
function NativePackagesSection({
  nativePackages,
}: {
  nativePackages: NativePackage[]
}) {
  return (
    <InspectorSection
      title="Native packages"
      description="Platform binaries unpacked for plugins on this device."
    >
      {nativePackages.length === 0 ? (
        <p className="text-foreground-alt/50 text-xs">
          No native packages on this device.
        </p>
      ) : (
        <ul className="divide-foreground/6 divide-y text-xs">
          {nativePackages.map((pkg) => (
            <li
              key={`${pkg.pluginId}:${pkg.distDir}`}
              className="py-2 first:pt-0 last:pb-0"
            >
              <div className="flex items-baseline justify-between gap-3">
                <span className="text-foreground truncate font-mono">
                  {pkg.pluginId}
                </span>
                <span className="text-foreground-alt/70 shrink-0">
                  {pkg.invalidated
                    ? 'Invalidated'
                    : pkg.materialized
                      ? 'Ready'
                      : 'Not unpacked'}
                </span>
              </div>
              {pkg.lastError && (
                <p className="text-destructive mt-0.5">{pkg.lastError}</p>
              )}
            </li>
          ))}
        </ul>
      )}
    </InspectorSection>
  )
}

// ReleaseInspector shows this build, the launcher's release and update
// state, the browser boot report, the runtime asset report, and native
// packages.
export function ReleaseInspector({
  build,
  recovery,
}: {
  build: AppBuildInfo
  recovery: RecoveryStatus | null
}) {
  const launcher = recovery?.launcher
  return (
    <div className="grid gap-3 @3xl:grid-cols-2">
      <BuildSection build={build} />
      <AppUpdateSection recovery={recovery} launcher={launcher} />
      <DaemonUpdateSection launcher={launcher} />
      <LauncherConfigSection launcher={launcher} />
      <BootSection boot={recovery?.boot} />
      <RuntimeAssetSection asset={recovery?.runtimeAsset} />
      <NativePackagesSection nativePackages={recovery?.nativePackages ?? []} />
    </div>
  )
}

// revisionFact formats a config revision with its source, or null when the
// launcher has not selected one.
function revisionFact(rev?: bigint, source?: string): string | null {
  if (!rev) {
    return null
  }
  return source ? `${rev} from ${source}` : String(rev)
}
