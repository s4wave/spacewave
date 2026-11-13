// The Development React setting's model, following
// bldr/web/bldr/ReactDev.tla: one reducer over the device setting, the cached
// generation, the copy into the cache and the build this document runs, and a
// view derived from it.
import { useCallback, useEffect, useReducer, useState } from 'react'

import {
  fillReactDevGeneration,
  hasReactDevGeneration,
  readReactDevRelease,
  readReactDevSetting,
  runningReactDev,
  writeReactDevSetting,
  type ReactDevRelease,
} from '@aptre/bldr'

// ReactDevFill is the copy of the release's development files into the cache.
export type ReactDevFill = 'idle' | 'fetching' | 'failed'

// ReactDevModel is the state of the Development React setting.
export interface ReactDevModel {
  // enabled is the device setting.
  enabled: boolean
  // cached is the React version of the complete cached generation, or empty.
  cached: string
  // fill is the state of the copy.
  fill: ReactDevFill
  // release is the release's React version.
  release: string
  // running is whether this document booted Development React.
  running: boolean
}

// ReactDevView names the state the person sees.
export type ReactDevView =
  | 'off'
  | 'fetching'
  | 'ready'
  | 'on'
  | 'absent'
  | 'mismatch'

// ReactDevEvent moves the model.
export type ReactDevEvent =
  | { type: 'checked'; cached: string }
  | { type: 'enable' }
  | { type: 'disable' }
  | { type: 'refetch' }
  | { type: 'filled' }
  | { type: 'fill-failed' }

// reactDevView derives the view, in the spec's order.
export function reactDevView(m: ReactDevModel): ReactDevView {
  if (m.fill === 'fetching') return 'fetching'
  if (!m.enabled) return 'off'
  if (m.running) return 'on'
  if (m.cached === m.release) return 'ready'
  if (m.fill === 'failed' || !m.cached) return 'absent'
  return 'mismatch'
}

// reactDevReducer applies one event.
export function reactDevReducer(
  m: ReactDevModel,
  e: ReactDevEvent,
): ReactDevModel {
  switch (e.type) {
    case 'checked':
      return { ...m, cached: e.cached }
    case 'enable':
      return {
        ...m,
        enabled: true,
        fill: m.cached === m.release ? 'idle' : 'fetching',
      }
    case 'disable':
      return { ...m, enabled: false, fill: 'idle' }
    case 'refetch':
      return m.enabled && m.fill !== 'fetching' ? { ...m, fill: 'fetching' } : m
    case 'filled':
      return m.fill === 'fetching'
        ? { ...m, fill: 'idle', cached: m.release }
        : m
    case 'fill-failed':
      return m.fill === 'fetching' ? { ...m, fill: 'failed' } : m
  }
}

// ReactDevControl is the setting's state and actions, or null in a shell
// without a service worker or a release without Development React.
export interface ReactDevControl {
  model: ReactDevModel
  view: ReactDevView
  dispatch: (e: ReactDevEvent) => void
  reload: () => void
}

// readRelease reads the release, or null where the setting cannot apply.
function readRelease(): ReactDevRelease | null {
  if (typeof document === 'undefined' || !('serviceWorker' in navigator)) {
    return null
  }
  try {
    return readReactDevRelease(document)
  } catch {
    return null
  }
}

// initModel reads the stored setting and the running build.
function initModel(release: ReactDevRelease | null): ReactDevModel {
  const setting = readReactDevSetting(localStorage)
  return {
    enabled: setting.enabled,
    cached: setting.version,
    fill: 'idle',
    release: release?.version ?? '',
    running: runningReactDev(),
  }
}

// useReactDev runs the Development React setting: it confirms the cached
// generation, copies the release's files when the model is fetching, and
// stores the setting the shell reads at the next boot.
export function useReactDev(): ReactDevControl | null {
  const [release] = useState(readRelease)
  const [model, dispatch] = useReducer(reactDevReducer, release, initModel)

  // Confirm the stored version still has a complete generation; clearing site
  // data or an eviction drops it without touching the setting.
  const stored = model.cached
  useEffect(() => {
    if (!release || !stored) return
    let live = true
    hasReactDevGeneration(caches, location.origin, stored).then(
      (has) => live && !has && dispatch({ type: 'checked', cached: '' }),
      () => live && dispatch({ type: 'checked', cached: '' }),
    )
    return () => {
      live = false
    }
  }, [release, stored])

  // Copy the release's files while fetching.
  const fetching = model.fill === 'fetching'
  useEffect(() => {
    if (!release || !fetching) return
    let live = true
    fillReactDevGeneration(
      caches,
      location.origin,
      release.version,
      release.source,
    ).then(
      () => live && dispatch({ type: 'filled' }),
      () => live && dispatch({ type: 'fill-failed' }),
    )
    return () => {
      live = false
    }
  }, [release, fetching])

  // Store the setting the shell reads.
  useEffect(() => {
    if (!release) return
    writeReactDevSetting(localStorage, {
      enabled: model.enabled,
      version: model.cached,
    })
  }, [release, model.enabled, model.cached])

  const reload = useCallback(() => location.reload(), [])
  if (!release) return null
  return { model, view: reactDevView(model), dispatch, reload }
}
