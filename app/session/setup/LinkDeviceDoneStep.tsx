import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  LuArrowRight,
  LuCircleCheck,
  LuLink,
  LuRefreshCw,
} from 'react-icons/lu'

import type { SessionListEntry } from '@s4wave/core/session/session.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

export interface LinkDeviceDoneStepProps {
  session: Session | null | undefined
  remotePeerId: string | null
  onDone: (entry?: SessionListEntry) => void
  onLinkMore: (entry?: SessionListEntry) => void
}

// LinkDeviceDoneStep reads the durable account attachment. File-copy progress is
// independent; a paired-device record never establishes either completion state.
export function LinkDeviceDoneStep({
  session,
  remotePeerId,
  onDone,
  onLinkMore,
}: LinkDeviceDoneStepProps) {
  const completion = useResource(
    async (signal) =>
      session && remotePeerId
        ? session.confirmPairing(remotePeerId, '', signal)
        : null,
    [session, remotePeerId],
  )
  const result = completion.loading ? null : completion.value
  const error = completion.error?.message

  if (error) {
    return (
      <div className="space-y-4 text-center">
        <h2 className="text-foreground text-sm font-medium">Pairing failed</h2>
        <p className="text-destructive text-xs">{error}</p>
        <button
          onClick={() => onLinkMore()}
          className="border-foreground/20 hover:border-foreground/40 flex h-10 w-full items-center justify-center gap-2 rounded-md border"
        >
          <LuRefreshCw className="text-foreground-alt size-4" />
          <span className="text-foreground text-sm">Try again</span>
        </button>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col items-center gap-3 text-center">
        <div className="bg-brand/10 flex size-12 items-center justify-center rounded-full">
          {result ? (
            <LuCircleCheck className="text-brand size-6" />
          ) : (
            <Spinner size="lg" variant="brand" />
          )}
        </div>
        <h2 className="text-foreground text-sm font-medium">
          {result ? 'Account connected' : 'Opening paired account…'}
        </h2>
        {result && (
          <p className="text-foreground-alt text-xs">
            Your Spaces are ready to open.
          </p>
        )}
      </div>
      {result && (
        <div className="flex gap-2">
          <button
            onClick={() => onLinkMore(result.sessionListEntry)}
            className="border-foreground/20 hover:border-foreground/40 flex h-10 flex-1 items-center justify-center gap-2 rounded-md border"
          >
            <LuLink className="text-foreground-alt size-4" />
            <span className="text-foreground text-sm">Link more</span>
          </button>
          <button
            onClick={() => onDone(result.sessionListEntry)}
            className="border-brand/30 bg-brand/10 hover:bg-brand/20 flex h-10 flex-1 items-center justify-center gap-2 rounded-md border"
          >
            <span className="text-foreground text-sm">Open account</span>
            <LuArrowRight className="text-foreground-alt size-4" />
          </button>
        </div>
      )}
    </div>
  )
}
