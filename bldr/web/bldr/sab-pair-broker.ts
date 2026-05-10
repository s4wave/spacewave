export type SabPairState = 'allocating' | 'open' | 'closing'

export interface SabPairMetadata {
  pairId: string
  key: string
  workerAId: string
  workerBId: string
  state: SabPairState
}

function normalizeWorkerIds(workerAId: string, workerBId: string): [string, string] {
  if (!workerAId || !workerBId) {
    throw new Error('SAB pair requires two worker ids')
  }
  if (workerAId === workerBId) {
    throw new Error('SAB pair requires two distinct workers')
  }
  return workerAId < workerBId
    ? [workerAId, workerBId]
    : [workerBId, workerAId]
}

function pairKey(workerAId: string, workerBId: string, pairId: string): string {
  return JSON.stringify([workerAId, workerBId, pairId])
}

// SabPairBroker owns WebDocument-local pair metadata. SharedArrayBuffer objects
// are intentionally excluded from this state.
export class SabPairBroker {
  private nextPairNumber = 1
  private readonly pairs = new Map<string, SabPairMetadata>()

  public allocate(workerAId: string, workerBId: string): SabPairMetadata {
    const [normalizedAId, normalizedBId] = normalizeWorkerIds(
      workerAId,
      workerBId,
    )
    const pairId = `sab-pair-${this.nextPairNumber++}`
    const key = pairKey(normalizedAId, normalizedBId, pairId)
    const pair: SabPairMetadata = {
      pairId,
      key,
      workerAId: normalizedAId,
      workerBId: normalizedBId,
      state: 'allocating',
    }
    this.pairs.set(key, pair)
    return { ...pair }
  }

  public markOpen(pairId: string): void {
    const pair = this.findPair(pairId)
    if (pair) {
      pair.state = 'open'
    }
  }

  public closePair(pairId: string): SabPairMetadata | undefined {
    const pair = this.findPair(pairId)
    if (!pair) {
      return undefined
    }
    pair.state = 'closing'
    this.pairs.delete(pair.key)
    return { ...pair }
  }

  public closePairForWorker(
    workerId: string,
    pairId: string,
  ): SabPairMetadata | undefined {
    const pair = this.findPair(pairId)
    if (
      !pair ||
      (pair.workerAId !== workerId && pair.workerBId !== workerId)
    ) {
      return undefined
    }
    pair.state = 'closing'
    this.pairs.delete(pair.key)
    return { ...pair }
  }

  public closeForWorker(workerId: string): SabPairMetadata[] {
    const closed: SabPairMetadata[] = []
    for (const pair of this.pairs.values()) {
      if (pair.workerAId === workerId || pair.workerBId === workerId) {
        pair.state = 'closing'
        this.pairs.delete(pair.key)
        closed.push({ ...pair })
      }
    }
    return closed
  }

  public closeAll(): void {
    for (const pair of this.pairs.values()) {
      pair.state = 'closing'
    }
    this.pairs.clear()
  }

  public snapshot(): SabPairMetadata[] {
    return Array.from(this.pairs.values(), (pair) => ({ ...pair }))
  }

  public get size(): number {
    return this.pairs.size
  }

  private findPair(pairId: string): SabPairMetadata | undefined {
    for (const pair of this.pairs.values()) {
      if (pair.pairId === pairId) {
        return pair
      }
    }
    return undefined
  }
}
