// ViewMode controls which content panel is displayed.
export type ViewMode =
  | 'files'
  | 'readme'
  | 'log'
  | 'commit'
  | 'workdir'
  | 'changes'

// RouteInfo describes the parsed viewer route.
export interface RouteInfo {
  mode: ViewMode
  ref: string | null
  subpath: string
  commitHash: string | null
}

// parseViewerRoute parses a GitHub-style viewer path into route components.
export function parseViewerRoute(viewerPath: string): RouteInfo {
  const segments = viewerPath.replace(/^\//, '').split('/').filter(Boolean)
  if (segments.length === 0) {
    return { mode: 'files', ref: null, subpath: '/', commitHash: null }
  }
  const action = segments[0]
  if ((action === 'tree' || action === 'blob') && segments.length >= 2) {
    const sub = segments.slice(2).join('/')
    return {
      mode: 'files',
      ref: segments[1],
      subpath: sub ? '/' + sub : '/',
      commitHash: null,
    }
  }
  if (action === 'readme' && segments.length >= 2) {
    return { mode: 'readme', ref: segments[1], subpath: '/', commitHash: null }
  }
  if (action === 'commits' && segments.length >= 2) {
    return { mode: 'log', ref: segments[1], subpath: '/', commitHash: null }
  }
  if (action === 'commit' && segments.length >= 2) {
    return {
      mode: 'commit',
      ref: segments[1],
      subpath: '/',
      commitHash: segments[1],
    }
  }
  if (action === 'workdir') {
    const sub = segments.slice(1).join('/')
    return {
      mode: 'workdir',
      ref: null,
      subpath: sub ? '/' + sub : '/',
      commitHash: null,
    }
  }
  if (action === 'changes') {
    return { mode: 'changes', ref: null, subpath: '/', commitHash: null }
  }
  return { mode: 'files', ref: null, subpath: '/', commitHash: null }
}
