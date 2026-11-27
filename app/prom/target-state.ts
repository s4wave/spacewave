import { isTargetStale } from '@s4wave/sdk/prom/prom.js'
import type { TargetInfo, TargetStatus } from '@s4wave/sdk/prom/prom.pb.js'

// TargetState is what a member sees of a Scrape target. The process binding
// is local to each Device, so approved says only whether this Device
// approved the target.
export type TargetState =
  // No Device has scraped the target, and this Device has not approved it.
  | { state: 'unapproved' }
  // This Device approved the target and has not committed a scrape of it yet,
  // or is standing by for the Device that does.
  | { state: 'waiting' }
  // The latest scrape succeeded.
  | { state: 'up'; status: TargetStatus }
  // The latest scrape failed.
  | { state: 'down'; status: TargetStatus }
  // The scraping Device stopped committing.
  | { state: 'stale'; status: TargetStatus }

// targetState derives the target's state from its info, whether this Device
// approved it, and the time.
export function targetState(
  info: TargetInfo | undefined,
  approved: boolean,
  myPeerId: string,
  nowMs: number,
): TargetState {
  const status = info?.status
  if (!info || !status)
    return approved ? { state: 'waiting' } : { state: 'unapproved' }
  if (isTargetStale(info, nowMs)) {
    // An approved Device stands by until the old writer goes stale, then
    // takes over. The approved Device is waiting for its own first scrape.
    if (approved && status.devicePeerId !== myPeerId)
      return { state: 'waiting' }
    return { state: 'stale', status }
  }
  return status.up ? { state: 'up', status } : { state: 'down', status }
}

// TargetAction is the one approval action a member can take on a target.
export type TargetAction = 'scrape-here' | 'take-over' | 'stop' | null

// targetAction returns the approval action offered in state: only a writer
// on a desktop Device changes approval, and taking over a live target is not
// offered.
export function targetAction(
  state: TargetState,
  approved: boolean,
  canApprove: boolean,
): TargetAction {
  if (!canApprove) return null
  if (approved) return 'stop'
  switch (state.state) {
    case 'unapproved':
    case 'waiting':
      return 'scrape-here'
    case 'stale':
      return 'take-over'
    case 'up':
    case 'down':
      return null
  }
}
