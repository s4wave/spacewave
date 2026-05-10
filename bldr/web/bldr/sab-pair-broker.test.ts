import { describe, expect, it } from 'vitest'

import { SabPairBroker } from './sab-pair-broker.js'

describe('SabPairBroker', () => {
  it('allocates normalized worker-pair metadata without SAB references', () => {
    const broker = new SabPairBroker()

    const pair = broker.allocate('worker-b', 'worker-a')

    expect(pair).toMatchObject({
      pairId: 'sab-pair-1',
      workerAId: 'worker-a',
      workerBId: 'worker-b',
      state: 'allocating',
    })
    expect(pair.key).toBe(
      JSON.stringify(['worker-a', 'worker-b', 'sab-pair-1']),
    )
    expect(Object.keys(pair)).not.toContain('aSab')
    expect(Object.keys(pair)).not.toContain('bSab')
    expect(broker.snapshot()).toEqual([pair])
  })

  it('allocates a fresh pair id for repeated worker pairs', () => {
    const broker = new SabPairBroker()

    const first = broker.allocate('worker-a', 'worker-b')
    const second = broker.allocate('worker-b', 'worker-a')

    expect(first.pairId).toBe('sab-pair-1')
    expect(second.pairId).toBe('sab-pair-2')
    expect(broker.snapshot()).toHaveLength(2)
  })

  it('rejects missing or identical worker ids', () => {
    const broker = new SabPairBroker()

    expect(() => broker.allocate('', 'worker-b')).toThrow(
      'SAB pair requires two worker ids',
    )
    expect(() => broker.allocate('worker-a', 'worker-a')).toThrow(
      'SAB pair requires two distinct workers',
    )
  })

  it('removes pair metadata by pair id and worker id', () => {
    const broker = new SabPairBroker()
    const first = broker.allocate('worker-a', 'worker-b')
    broker.allocate('worker-a', 'worker-c')

    broker.markOpen(first.pairId)
    expect(broker.closePair(first.pairId)).toMatchObject({
      pairId: first.pairId,
      state: 'closing',
    })

    expect(broker.snapshot()).toMatchObject([
      {
        pairId: 'sab-pair-2',
        workerAId: 'worker-a',
        workerBId: 'worker-c',
      },
    ])

    expect(broker.closeForWorker('worker-c')).toMatchObject([
      {
        pairId: 'sab-pair-2',
        state: 'closing',
      },
    ])
    expect(broker.snapshot()).toEqual([])
  })

  it('ignores close-pair requests from unrelated workers', () => {
    const broker = new SabPairBroker()
    const pair = broker.allocate('worker-a', 'worker-b')

    expect(broker.closePairForWorker('worker-c', pair.pairId)).toBeUndefined()
    expect(broker.snapshot()).toHaveLength(1)

    expect(broker.closePairForWorker('worker-b', pair.pairId)).toMatchObject({
      pairId: pair.pairId,
      state: 'closing',
    })
    expect(broker.snapshot()).toEqual([])
  })
})
