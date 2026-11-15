import { useEffect, useId } from 'react'

import type {
  SharedObjectHealth,
  SORejectedEdit,
  SOReplayWait,
} from '@s4wave/core/sobject/sobject.pb.js'
import { toast } from '@s4wave/web/ui/toaster.js'

// SharedObjectSyncNotice presents peer recovery, rejected edits, a wrong
// checkpoint and a replay wait without interrupting local content.
// SharedObjectSyncNotice.tla specifies the replay wait.
export function SharedObjectSyncNotice({
  health,
}: {
  health?: SharedObjectHealth
}) {
  const id = useId()
  const recovery = (health?.syncRecoveryPeerIds?.length ?? 0) > 0
  const denied = (health?.syncDeniedPeerIds?.length ?? 0) > 0
  const mismatch = !!health?.checkpointMismatch
  const { title: waitTitle, description: waitDescription } =
    describeReplayWait(health?.replayWait) ?? {}

  useEffect(() => {
    if (!recovery && !denied) return
    toast.warning('Direct sync needs attention', {
      id,
      description: recovery
        ? 'Update both devices. If sync still cannot reconnect, ask the owner for a new invite. You can keep using local content.'
        : 'A connected device declined to sync this Space. Ask the owner to confirm your access. You can keep using local content.',
      duration: Infinity,
      closeButton: true,
    })
    return () => {
      toast.dismiss(id)
    }
  }, [denied, id, recovery])

  useEffect(() => {
    if (!mismatch) return
    const mismatchID = id + ':checkpoint'
    toast.warning("This Space's history doesn't match this device", {
      id: mismatchID,
      description:
        "The owner's device saved a summary of this Space that differs from what this device worked out. Your content is still here. Ask the owner to check their device.",
      duration: Infinity,
      closeButton: true,
    })
    return () => {
      toast.dismiss(mismatchID)
    }
  }, [id, mismatch])

  useEffect(() => {
    if (!waitTitle) return
    const waitID = id + ':replay-wait'
    toast.info(waitTitle, {
      id: waitID,
      description: waitDescription,
      duration: Infinity,
      closeButton: true,
    })
    return () => {
      toast.dismiss(waitID)
    }
  }, [id, waitDescription, waitTitle])

  return (health?.rejectedEdits ?? []).map((edit) => {
    const hash = opHashKey(edit.opHash)
    return (
      <RejectedEditNotice
        key={hash}
        id={id + ':' + hash}
        description={describeRejectedEdit(edit)}
      />
    )
  })
}

// RejectedEditNotice shows a dismissible toast for one rejected edit while it
// stays in the health.
function RejectedEditNotice({
  id,
  description,
}: {
  id: string
  description: string
}) {
  useEffect(() => {
    toast.warning("An edit didn't apply", {
      id,
      description,
      duration: Infinity,
      closeButton: true,
    })
    return () => {
      toast.dismiss(id)
    }
  }, [description, id])

  return null
}

// describeRejectedEdit says in plain words why an edit did not apply.
function describeRejectedEdit(edit: SORejectedEdit): string {
  if ((edit.lostToPeerIds?.length ?? 0) > 0) {
    return "Another member's change reached the Space first, so this edit no longer applies. Your other edits are kept."
  }
  return `This edit no longer applies because ${edit.reason || 'the Space rejected it'}. Your other edits are kept.`
}

// describeReplayWait says in plain words what this device's replay waits for,
// or returns undefined when it waits for nothing.
function describeReplayWait(
  wait?: SOReplayWait,
): { title: string; description: string } | undefined {
  if (!wait?.opTypeId) return undefined
  const later =
    'You are seeing the Space as it was before that change. Edits you make now are saved and apply once it catches up.'
  if (wait.state?.case === 'approval') {
    const plugins = wait.state.value.pluginIds ?? []
    const named = plugins.length > 0 ? plugins.join(', ') : 'a plugin'
    return {
      title: 'This Space is waiting for a plugin approval',
      description: `A change in this Space needs ${named}, which is not approved on this device. ${later}`,
    }
  }
  return {
    title: 'This Space is waiting for an app it needs',
    description: `A change in this Space needs an app that has not started on this device yet. ${later}`,
  }
}

// opHashKey encodes an operation hash as lowercase hex.
function opHashKey(hash?: Uint8Array): string {
  return Array.from(hash ?? [], (b) => b.toString(16).padStart(2, '0')).join('')
}
