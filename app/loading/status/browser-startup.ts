import { useSyncExternalStore } from 'react'

import type { LoadingView } from '@s4wave/web/ui/loading/types.js'

import {
  readBrowserBootStatus,
  readBrowserStartupRevision,
  readBrowserStartupMarks,
  subscribeBrowserBootStatus,
  useBrowserBootStatus,
  type BrowserBootStatus,
} from '@s4wave/app/prerender/boot-status.js'
import {
  projectBrowserStartup,
  projectBrowserStartupView as projectBrowserStartupViewModel,
  type BrowserStartupProjection,
} from './browser-startup-model.js'

export function projectBrowserStartupView(
  status: BrowserBootStatus,
  marks = readBrowserStartupMarks(),
): LoadingView {
  return projectBrowserStartupViewModel(status, marks)
}

export function readBrowserStartupProjection(): BrowserStartupProjection {
  return projectBrowserStartup(
    readBrowserBootStatus(),
    readBrowserStartupMarks(),
  )
}

export function readBrowserStartupView(): LoadingView {
  return readBrowserStartupProjection().view
}

export function useBrowserStartupView(): LoadingView {
  return useBrowserStartupProjection().view
}

export function useBrowserStartupProjection(): BrowserStartupProjection {
  const revision = useSyncExternalStore(
    subscribeBrowserBootStatus,
    readBrowserStartupRevision,
    readBrowserStartupRevision,
  )
  void revision
  return projectBrowserStartup(
    useBrowserBootStatus(),
    readBrowserStartupMarks(),
  )
}
