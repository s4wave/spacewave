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
  // line is the 1-based line a file route selects with a #L<n> suffix, as
  // GitHub does, so a link can open a file at one line.
  line?: number
}

// lineSuffix matches a file route's #L<n> line anchor.
const lineSuffix = /#L([1-9][0-9]*)$/

// parseViewerRoute parses a GitHub-style viewer path into route components.
export function parseViewerRoute(viewerPath: string): RouteInfo {
  const segments = viewerPath.replace(/^\//, '').split('/').filter(Boolean)
  if (segments.length === 0) {
    return { mode: 'files', ref: null, subpath: '/', commitHash: null }
  }
  const action = segments[0]
  if ((action === 'tree' || action === 'blob') && segments.length >= 2) {
    const joined = segments.slice(2).join('/')
    const anchor = lineSuffix.exec(joined)
    const sub = anchor ? joined.slice(0, anchor.index) : joined
    return {
      mode: 'files',
      ref: segments[1],
      subpath: sub ? '/' + sub : '/',
      commitHash: null,
      ...(anchor && { line: Number(anchor[1]) }),
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
