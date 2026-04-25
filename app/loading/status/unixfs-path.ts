import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

import type { LoadingView } from '@s4wave/web/ui/loading/types.js'

interface UnixFSPathViewInput {
  root: Resource<unknown> | null | undefined
  lookup: Resource<unknown> | null | undefined
  stat: Resource<unknown> | null | undefined
  entries: Resource<unknown> | null | undefined
  path?: string
  onRetry?: () => void
  onCancel?: () => void
}

// toUnixFSPathView combines the four sequential Resource handles used by
// UnixFSBrowser (root, lookup, stat, readdir) into a single staged
// LoadingView. The returned detail line reflects the lowest-numbered stage
// that is still in-flight; when all four resolve the view switches to
// 'synced' (primitive unmounts). Errors on any stage surface as 'error'
// with the stage name in the detail line.
export function toUnixFSPathView(input: UnixFSPathViewInput): LoadingView {
  const { root, lookup, stat, entries, path, onRetry, onCancel } = input
  const stages: Array<{
    label: string
    detail: string
    resource: Resource<unknown> | null | undefined
  }> = [
    {
      label: 'Opening root',
      detail: 'Mounting the UnixFS root.',
      resource: root,
    },
    {
      label: 'Resolving path',
      detail: 'Looking up the requested path.',
      resource: lookup,
    },
    {
      label: 'Reading metadata',
      detail: 'Reading the entry metadata.',
      resource: stat,
    },
    {
      label: 'Listing entries',
      detail: 'Listing directory entries.',
      resource: entries,
    },
  ]

  for (const stage of stages) {
    if (stage.resource?.error) {
      return {
        state: 'error',
        title: stage.label + ' failed',
        detail: path ? `Path: ${path}` : undefined,
        error: stage.resource.error.message,
        onRetry: onRetry ?? stage.resource.retry,
        onCancel,
      }
    }
  }
  for (const stage of stages) {
    const r = stage.resource
    if (!r || r.loading || r.value === null) {
      return {
        state: 'active',
        title: 'Loading files',
        detail: path ? `${stage.detail} Path: ${path}` : stage.detail,
        onCancel,
      }
    }
  }
  return {
    state: 'synced',
    title: 'Files loaded',
    detail: path ? `Path: ${path}` : undefined,
  }
}
