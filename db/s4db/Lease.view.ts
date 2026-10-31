// The lease seen from process P1, whose callers a and b share one handle,
// while c in process P2 contends for the same lock byte.

type Lease = { owner: string; claimed: string[]; pc: Record<string, string> }

export const title = 'One lock byte: P1 leases it while P2 contends'

// P1's handle, from the state of the one P1 caller with a claim.
const handle: Record<string, string> = {
  try: 'P1 tries',
  wait: 'P1 waits',
  held: 'P1 holds the lease',
  abandoned: 'P1 abandoned its wait',
  granted: 'P1 unlocks its abandoned lock',
}

const p1 = (v: Lease) =>
  ['a', 'b'].map((g) => v.pc[g]).find((s) => s !== 'idle') ?? 'idle'

export const state = (v: Lease) => {
  const p = handle[p1(v)] ?? 'P1 idle'
  // A try ends at once, and its arrows say whether P2 held the lock.
  return v.owner === 'P2' && p1(v) !== 'try' ? `${p}, P2 holds the lock` : p
}

export const step = (action: string, from: Lease, to: Lease) => {
  if (from.owner !== 'P2' && to.owner === 'P2') return 'P2 locks'
  if (from.owner === 'P2' && to.owner !== 'P2') return 'P2 unlocks'
  if (p1(from) === p1(to)) return null
  switch (action) {
    case 'Claim':
      return p1(to) === 'try' ? 'TryLease' : 'WaitLease'
    case 'Try':
      return to.owner === 'P1'
        ? 'the lock is free'
        : 'P2 has it, drop the claim'
    case 'Grant':
    case 'AbandonedGrant':
      return 'the lock is granted'
    case 'Abandon':
      return 'the context ends'
    case 'AbandonedRelease':
      return 'unlock and drop the claim'
    default:
      return 'Release'
  }
}
