import type { ComponentProps } from 'react'
import { LuCircleAlert, LuRotateCw } from 'react-icons/lu'

import { UnixFSBrowserShell } from './UnixFSBrowserShell.js'

type UnixFSBrowserShellProps = Omit<
  ComponentProps<typeof UnixFSBrowserShell>,
  'children'
>

type UnixFSBrowserUnavailableStateProps =
  | {
      kind: 'error'
      shellProps: UnixFSBrowserShellProps
      error: Error
      onRetry: () => void
    }
  | {
      kind: 'not-found'
      unixfsId: string
    }

export function UnixFSBrowserUnavailableState(
  props: UnixFSBrowserUnavailableStateProps,
) {
  if (props.kind === 'error') {
    return (
      <UnixFSBrowserShell {...props.shellProps}>
        <div className="border-destructive/20 bg-destructive/5 flex items-center gap-2 border-b px-3 py-1.5">
          <LuCircleAlert className="text-destructive size-3.5 shrink-0" />
          <p className="text-foreground/80 min-w-0 flex-1 truncate text-xs">
            Error loading files: {props.error.message ?? 'Unknown error'}
          </p>
          <button
            type="button"
            onClick={props.onRetry}
            className="text-foreground-alt hover:text-foreground inline-flex items-center gap-1 text-xs font-medium transition-colors"
          >
            <LuRotateCw className="size-3" />
            Retry
          </button>
        </div>
        <div className="bg-file-back flex min-h-0 flex-1 overflow-hidden" />
      </UnixFSBrowserShell>
    )
  }

  return (
    <div
      data-testid="unixfs-browser"
      className="flex h-full w-full flex-col overflow-hidden"
    >
      <div className="bg-file-back flex min-h-0 flex-1 flex-col items-center justify-center overflow-hidden">
        <div className="text-foreground-alt text-sm">
          UnixFS object not found
        </div>
        <div className="text-foreground-alt/70 mt-1 text-xs">
          Object: {props.unixfsId || 'none'}
        </div>
      </div>
    </div>
  )
}
