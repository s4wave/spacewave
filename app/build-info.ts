import { useState } from 'react'

interface RawBuildInfo {
  mainVersion?: string
  version?: string
  goVersion?: string
  goos?: string
  goarch?: string
  runtimeLabel?: string
  cornerLabel?: string
  browserGenerationId?: string
}

export interface AppBuildInfo {
  mainVersion: string
  version: string
  goVersion: string
  goos: string
  goarch: string
  runtimeLabel: string
  cornerLabel: string
  browserGenerationId: string
}

declare global {
  var __BLDR_BUILD_INFO__: RawBuildInfo | undefined
  var __swGenerationId: string | undefined
}

export const FALLBACK_APP_BUILD_INFO: AppBuildInfo = {
  mainVersion: '',
  version: 'dev',
  goVersion: '',
  goos: '',
  goarch: '',
  runtimeLabel: '',
  cornerLabel: 'dev',
  browserGenerationId: '',
}

function readBuildInfo(): AppBuildInfo {
  const raw = globalThis.__BLDR_BUILD_INFO__
  if (!raw) return FALLBACK_APP_BUILD_INFO
  const version = raw.version || 'dev'
  const goVersion = raw.goVersion || ''
  const runtimeLabel =
    raw.runtimeLabel ||
    (goVersion && raw.goos && raw.goarch ?
      `${goVersion} on ${raw.goos}/${raw.goarch}`
    : '')
  return {
    mainVersion: raw.mainVersion || '',
    version,
    goVersion,
    goos: raw.goos || '',
    goarch: raw.goarch || '',
    runtimeLabel,
    cornerLabel:
      raw.cornerLabel || (goVersion ? `${version}@${goVersion}` : version),
    browserGenerationId:
      globalThis.__swGenerationId || raw.browserGenerationId || '',
  }
}

export function getAppBuildInfo(): AppBuildInfo {
  return readBuildInfo()
}

// useAppBuildInfo reads build info after mount to avoid prerender hydration mismatch.
export function useAppBuildInfo(): AppBuildInfo {
  const [info] = useState(readBuildInfo)
  return info
}
