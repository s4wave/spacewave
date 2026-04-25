import { GITHUB_RELEASES_URL } from '@s4wave/app/github.js'
import type {
  DetectedPlatform,
  PlatformArch,
  PlatformOS,
} from '@s4wave/web/platform/detect-platform.js'

// DownloadOS aliases the shared PlatformOS enum.
export type DownloadOS = PlatformOS

// DownloadArch aliases the shared PlatformArch enum.
export type DownloadArch = PlatformArch

// DownloadKind discriminates between desktop installers and standalone
// CLI binary artifacts that ship in the same release.
export type DownloadKind = 'installer' | 'cli'

// DownloadExt enumerates every artifact extension in DOWNLOAD_MANIFEST.
// Installer extensions: dmg, zip, msix, AppImage. CLI extensions: tar.gz
// (Linux archives) and zip (macOS notarized archives, Windows archives).
export type DownloadExt = 'dmg' | 'zip' | 'msix' | 'AppImage' | 'tar.gz'

// DownloadEntry describes one downloadable release artifact. The kind
// discriminator distinguishes desktop installers from standalone CLI
// binaries that share the same host matrix.
export interface DownloadEntry {
  kind: DownloadKind
  os: DownloadOS
  arch: DownloadArch
  osLabel: string
  archLabel: string
  filename: string
  ext: DownloadExt
  url: string
  unsigned?: boolean
}

// assetUrl builds the public release asset URL for a given filename.
// Kind-agnostic: the same GitHub release ships installers and CLI
// artifacts side by side.
export function assetUrl(filename: string): string {
  return `${GITHUB_RELEASES_URL}/download/${filename}`
}

// resolvePrimaryEntry returns the manifest entry matching the detected
// platform, or null when detection missed or the tuple is absent from
// the manifest. Optionally filtered by kind so installer and CLI
// sections can each pick their own primary entry from a shared manifest.
export function resolvePrimaryEntry(
  detected: DetectedPlatform | null,
  manifest: readonly DownloadEntry[],
  kind?: DownloadKind,
): DownloadEntry | null {
  if (!detected) return null
  return (
    manifest.find(
      (e) =>
        e.os === detected.os &&
        e.arch === detected.arch &&
        (kind === undefined || e.kind === kind),
    ) ?? null
  )
}

// DOWNLOAD_MANIFEST is the hardcoded list of release assets. Both
// desktop installers and standalone CLI binaries ship for the same host
// matrix from the same release tag. URLs point at the latest GitHub
// release. Windows installer entries carry unsigned: true during the
// Azure Trusted Signing identity-validation period (tracked in company
// shipyard/alpha.org); flip to .msix and drop the flag once signing
// clears. CLI artifacts are signed where platform signing is available
// (macOS zip, Windows zip); Linux CLI artifacts ship as tar.gz archives.
export const DOWNLOAD_MANIFEST: readonly DownloadEntry[] = [
  {
    kind: 'installer',
    os: 'macos',
    arch: 'arm64',
    osLabel: 'macOS',
    archLabel: 'Apple Silicon',
    filename: 'spacewave-macos-arm64.dmg',
    ext: 'dmg',
    url: assetUrl('spacewave-macos-arm64.dmg'),
  },
  {
    kind: 'installer',
    os: 'macos',
    arch: 'amd64',
    osLabel: 'macOS',
    archLabel: 'Intel',
    filename: 'spacewave-macos-amd64.dmg',
    ext: 'dmg',
    url: assetUrl('spacewave-macos-amd64.dmg'),
  },
  {
    kind: 'installer',
    os: 'windows',
    arch: 'amd64',
    osLabel: 'Windows',
    archLabel: 'x86_64',
    filename: 'spacewave-windows-amd64.zip',
    ext: 'zip',
    url: assetUrl('spacewave-windows-amd64.zip'),
    unsigned: true,
  },
  {
    kind: 'installer',
    os: 'windows',
    arch: 'arm64',
    osLabel: 'Windows',
    archLabel: 'ARM64',
    filename: 'spacewave-windows-arm64.zip',
    ext: 'zip',
    url: assetUrl('spacewave-windows-arm64.zip'),
    unsigned: true,
  },
  {
    kind: 'installer',
    os: 'linux',
    arch: 'amd64',
    osLabel: 'Linux',
    archLabel: 'x86_64',
    filename: 'spacewave-linux-amd64.AppImage',
    ext: 'AppImage',
    url: assetUrl('spacewave-linux-amd64.AppImage'),
  },
  {
    kind: 'installer',
    os: 'linux',
    arch: 'arm64',
    osLabel: 'Linux',
    archLabel: 'ARM64',
    filename: 'spacewave-linux-arm64.AppImage',
    ext: 'AppImage',
    url: assetUrl('spacewave-linux-arm64.AppImage'),
  },
  {
    kind: 'cli',
    os: 'macos',
    arch: 'arm64',
    osLabel: 'macOS',
    archLabel: 'Apple Silicon',
    filename: 'spacewave-cli-macos-arm64.zip',
    ext: 'zip',
    url: assetUrl('spacewave-cli-macos-arm64.zip'),
  },
  {
    kind: 'cli',
    os: 'macos',
    arch: 'amd64',
    osLabel: 'macOS',
    archLabel: 'Intel',
    filename: 'spacewave-cli-macos-amd64.zip',
    ext: 'zip',
    url: assetUrl('spacewave-cli-macos-amd64.zip'),
  },
  {
    kind: 'cli',
    os: 'linux',
    arch: 'amd64',
    osLabel: 'Linux',
    archLabel: 'x86_64',
    filename: 'spacewave-cli-linux-amd64.tar.gz',
    ext: 'tar.gz',
    url: assetUrl('spacewave-cli-linux-amd64.tar.gz'),
  },
  {
    kind: 'cli',
    os: 'linux',
    arch: 'arm64',
    osLabel: 'Linux',
    archLabel: 'ARM64',
    filename: 'spacewave-cli-linux-arm64.tar.gz',
    ext: 'tar.gz',
    url: assetUrl('spacewave-cli-linux-arm64.tar.gz'),
  },
  {
    kind: 'cli',
    os: 'windows',
    arch: 'amd64',
    osLabel: 'Windows',
    archLabel: 'x86_64',
    filename: 'spacewave-cli-windows-amd64.zip',
    ext: 'zip',
    url: assetUrl('spacewave-cli-windows-amd64.zip'),
  },
  {
    kind: 'cli',
    os: 'windows',
    arch: 'arm64',
    osLabel: 'Windows',
    archLabel: 'ARM64',
    filename: 'spacewave-cli-windows-arm64.zip',
    ext: 'zip',
    url: assetUrl('spacewave-cli-windows-arm64.zip'),
  },
]
