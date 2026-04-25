// PlatformOS enumerates the desktop operating systems we detect.
export type PlatformOS = 'macos' | 'windows' | 'linux'

// PlatformArch enumerates the CPU architectures we detect.
export type PlatformArch = 'amd64' | 'arm64'

// DetectedPlatform is the OS + arch pair resolved from a Navigator object.
export interface DetectedPlatform {
  os: PlatformOS
  arch: PlatformArch
}

function getUAPlatform(nav: Navigator): string | undefined {
  const uaData = (nav as { userAgentData?: { platform?: string } })
    .userAgentData
  return uaData?.platform
}

function detectOS(nav: Navigator): PlatformOS | null {
  const platform = getUAPlatform(nav)
  if (platform) {
    const p = platform.toLowerCase()
    if (p.includes('mac')) return 'macos'
    if (p.includes('windows')) return 'windows'
    if (p.includes('linux')) return 'linux'
  }
  const ua = nav.userAgent
  if (/Mac OS X|Macintosh/i.test(ua)) return 'macos'
  if (/Windows NT/i.test(ua)) return 'windows'
  // Android reports Linux in UA; exclude it.
  if (/Linux/i.test(ua) && !/Android/i.test(ua)) return 'linux'
  return null
}

// detectArch resolves the CPU architecture from the user agent string.
// macOS UAs no longer include architecture, so recent Macs default to arm64.
// Windows UAs typically spoof x64 without getHighEntropyValues, so arm64 is
// only detected when explicitly advertised.
function detectArch(nav: Navigator, os: PlatformOS): PlatformArch {
  const ua = nav.userAgent
  if (/arm64|aarch64/i.test(ua)) return 'arm64'
  if (/x86_64|Win64|WOW64|x64/i.test(ua)) return 'amd64'
  if (os === 'macos') return 'arm64'
  return 'amd64'
}

// detectPlatform resolves the desktop platform from a Navigator object. Uses
// userAgentData.platform when available, falls back to a userAgent regex.
// Returns null when the OS cannot be determined.
export function detectPlatform(nav: Navigator): DetectedPlatform | null {
  const os = detectOS(nav)
  if (!os) return null
  return { os, arch: detectArch(nav, os) }
}
