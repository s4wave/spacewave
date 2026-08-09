import { describe, expect, it, vi } from 'vitest'

import type { Session } from '@s4wave/sdk/session/session.js'

import { ManagedBillingAccountsStore } from './useManagedBillingAccounts.js'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, reject, resolve }
}

function buildStore(spacewave: Record<string, unknown>) {
  return new ManagedBillingAccountsStore({ spacewave } as unknown as Session)
}

describe('ManagedBillingAccountsStore', () => {
  it('publishes one refreshed snapshot to every consumer after assignment', async () => {
    const listManagedBillingAccounts = vi
      .fn()
      .mockResolvedValueOnce({ accounts: [{ id: 'ba_1', assignees: [] }] })
      .mockResolvedValueOnce({
        accounts: [
          {
            id: 'ba_1',
            assignees: [{ ownerType: 'account', ownerId: 'acct_1' }],
          },
        ],
      })
    const assignBillingAccount = vi.fn().mockResolvedValue({})
    const store = buildStore({
      assignBillingAccount,
      listManagedBillingAccounts,
    })
    await store.refresh()
    const first = vi.fn()
    const second = vi.fn()
    const unsubscribeFirst = store.subscribe(first)
    const unsubscribeSecond = store.subscribe(second)

    first.mockClear()
    second.mockClear()
    await store.assign('ba_1', 'account', 'acct_1')

    await vi.waitFor(() =>
      expect(store.getSnapshot().data?.accounts?.[0]?.assignees).toEqual([
        { ownerType: 'account', ownerId: 'acct_1' },
      ]),
    )
    expect(first).toHaveBeenCalled()
    expect(second).toHaveBeenCalled()
    expect(assignBillingAccount).toHaveBeenCalledWith(
      'ba_1',
      'account',
      'acct_1',
      expect.any(AbortSignal),
    )
    unsubscribeFirst()
    unsubscribeSecond()
  })

  it('continues initial and mutation publication when the listener reporter throws', async () => {
    const listenerFailure = new Error('listener failed')
    const reporterFailure = new Error('reporter failed')
    const listManagedBillingAccounts = vi
      .fn()
      .mockResolvedValueOnce({
        accounts: [{ id: 'ba_1', displayName: 'Initial' }],
      })
      .mockResolvedValueOnce({
        accounts: [{ id: 'ba_1', displayName: 'Updated' }],
      })
    const renameBillingAccount = vi.fn().mockResolvedValue({})
    const store = buildStore({
      listManagedBillingAccounts,
      renameBillingAccount,
    })
    const report = vi.spyOn(console, 'error').mockImplementation(() => {
      throw reporterFailure
    })
    const throwing = vi.fn(() => {
      throw listenerFailure
    })
    const later = vi.fn()
    const unsubscribeThrowing = store.subscribe(throwing)
    const unsubscribeLater = store.subscribe(later)

    try {
      await vi.waitFor(() =>
        expect(store.getSnapshot().data?.accounts?.[0]?.displayName).toBe(
          'Initial',
        ),
      )
      expect(listManagedBillingAccounts).toHaveBeenCalledOnce()

      await expect(store.rename('ba_1', 'Updated')).resolves.toEqual({})

      expect(renameBillingAccount).toHaveBeenCalledOnce()
      expect(listManagedBillingAccounts).toHaveBeenCalledTimes(2)
      expect(later).toHaveBeenCalledTimes(3)
      expect(store.getSnapshot()).toMatchObject({
        data: { accounts: [{ id: 'ba_1', displayName: 'Updated' }] },
        loading: false,
        error: null,
      })
      expect(report).toHaveBeenCalledWith(
        'Managed billing account listener failed:',
        listenerFailure,
      )
    } finally {
      unsubscribeThrowing()
      unsubscribeLater()
      report.mockRestore()
    }
  })

  it('keeps a mutation error and does not publish a false refresh', async () => {
    const failure = new Error('assignment rejected')
    const store = buildStore({
      assignBillingAccount: vi.fn().mockRejectedValue(failure),
      listManagedBillingAccounts: vi.fn().mockResolvedValue({ accounts: [] }),
    })

    await expect(store.assign('ba_1', 'account', 'acct_1')).rejects.toBe(
      failure,
    )
    expect(store.getSnapshot().data).toBeNull()
  })

  it('aborts in-flight mutation and ignores stale list completion after release', async () => {
    const load = deferred<{ accounts: [] }>()
    let mutationSignal: AbortSignal | undefined
    const mutation = deferred<{}>()
    const store = buildStore({
      assignBillingAccount: vi.fn(
        (_baId, _ownerType, _ownerId, signal: AbortSignal) => {
          mutationSignal = signal
          return mutation.promise
        },
      ),
      listManagedBillingAccounts: vi.fn(() => load.promise),
    })
    const listener = vi.fn()
    const unsubscribe = store.subscribe(listener)
    const result = store.assign('ba_1', 'account', 'acct_1')

    unsubscribe()
    expect(mutationSignal?.aborted).toBe(true)
    mutation.resolve({})
    load.resolve({ accounts: [] })
    await expect(result).rejects.toBeDefined()
    expect(store.getSnapshot().data).toBeNull()
  })
  it('reloads a stale snapshot when a consumer returns after abort', async () => {
    const first = deferred<{ accounts: [] }>()
    const listManagedBillingAccounts = vi
      .fn()
      .mockImplementationOnce(() => first.promise)
      .mockResolvedValueOnce({ accounts: [{ id: 'ba_fresh' }] })
    const store = buildStore({ listManagedBillingAccounts })
    const unsubscribe = store.subscribe(vi.fn())
    unsubscribe()
    first.resolve({ accounts: [] })

    const unsubscribeNext = store.subscribe(vi.fn())
    await vi.waitFor(() =>
      expect(store.getSnapshot().data?.accounts?.[0]?.id).toBe('ba_fresh'),
    )
    expect(listManagedBillingAccounts).toHaveBeenCalledTimes(2)
    unsubscribeNext()
  })

  it('runs mutations in invocation order when later work resolves first', async () => {
    const first = deferred<{}>()
    const second = deferred<{}>()
    const renameBillingAccount = vi.fn((_id: string, displayName: string) =>
      displayName === 'first' ? first.promise : second.promise,
    )
    const listManagedBillingAccounts = vi
      .fn()
      .mockResolvedValueOnce({
        accounts: [{ id: 'ba_1', displayName: 'first' }],
      })
      .mockResolvedValueOnce({
        accounts: [{ id: 'ba_1', displayName: 'second' }],
      })
    const store = buildStore({
      listManagedBillingAccounts,
      renameBillingAccount,
    })

    const firstResult = store.rename('ba_1', 'first')
    const secondResult = store.rename('ba_1', 'second')
    second.resolve({})
    await Promise.resolve()

    expect(renameBillingAccount.mock.calls.map((call) => call[1])).toEqual([
      'first',
    ])
    first.resolve({})
    await firstResult
    await secondResult
    await vi.waitFor(() =>
      expect(store.getSnapshot().data?.accounts?.[0]?.displayName).toBe(
        'second',
      ),
    )

    expect(renameBillingAccount.mock.calls.map((call) => call[1])).toEqual([
      'first',
      'second',
    ])
    expect(listManagedBillingAccounts).toHaveBeenCalledTimes(2)
    expect(store.getSnapshot().data?.accounts?.[0]?.displayName).toBe('second')
  })

  it('removes a queued mutation when its caller aborts', async () => {
    const running = deferred<{}>()
    const renameBillingAccount = vi.fn(() => running.promise)
    const store = buildStore({
      listManagedBillingAccounts: vi.fn().mockResolvedValue({ accounts: [] }),
      renameBillingAccount,
    })
    const firstResult = store.rename('ba_1', 'first')
    const controller = new AbortController()
    const queuedResult = store.rename('ba_1', 'second', controller.signal)

    controller.abort()
    await expect(queuedResult).rejects.toMatchObject({ name: 'AbortError' })
    running.resolve({})
    await firstResult

    expect(renameBillingAccount).toHaveBeenCalledTimes(1)
  })

  it('aborts a running mutation and continues with the next eligible item', async () => {
    let runningSignal: AbortSignal | undefined
    const renameBillingAccount = vi
      .fn()
      .mockImplementationOnce(
        (_id: string, _name: string, signal: AbortSignal) =>
          new Promise((_, reject) => {
            runningSignal = signal
            signal.addEventListener('abort', () => reject(signal.reason), {
              once: true,
            })
          }),
      )
      .mockResolvedValueOnce({})
    const store = buildStore({
      listManagedBillingAccounts: vi.fn().mockResolvedValue({ accounts: [] }),
      renameBillingAccount,
    })
    const controller = new AbortController()
    const aborted = store.rename('ba_1', 'first', controller.signal)
    const next = store.rename('ba_1', 'second')

    controller.abort()
    expect(runningSignal?.aborted).toBe(true)
    await expect(aborted).rejects.toMatchObject({ name: 'AbortError' })
    await next

    expect(renameBillingAccount.mock.calls.map((call) => call[1])).toEqual([
      'first',
      'second',
    ])
  })

  it('continues after a failed mutation and refreshes only the success', async () => {
    const failure = new Error('rename rejected')
    const renameBillingAccount = vi
      .fn()
      .mockRejectedValueOnce(failure)
      .mockResolvedValueOnce({})
    const listManagedBillingAccounts = vi
      .fn()
      .mockResolvedValue({ accounts: [{ id: 'ba_1', displayName: 'second' }] })
    const store = buildStore({
      listManagedBillingAccounts,
      renameBillingAccount,
    })

    const failed = store.rename('ba_1', 'first')
    const succeeded = store.rename('ba_1', 'second')
    await expect(failed).rejects.toBe(failure)
    await succeeded

    expect(listManagedBillingAccounts).toHaveBeenCalledTimes(1)
    expect(store.getSnapshot().data?.accounts?.[0]?.displayName).toBe('second')
  })

  it('returns a created ID so checkout proceeds when snapshot refresh fails', async () => {
    const refreshFailure = new Error('list unavailable')
    const createBillingAccount = vi.fn().mockResolvedValue('ba_created')
    const createCheckoutSession = vi.fn().mockResolvedValue({
      checkoutUrl: 'https://checkout.example/test',
    })
    const listManagedBillingAccounts = vi
      .fn()
      .mockRejectedValueOnce(refreshFailure)
      .mockResolvedValueOnce({ accounts: [{ id: 'ba_created' }] })
    const store = buildStore({
      createBillingAccount,
      createCheckoutSession,
      listManagedBillingAccounts,
    })

    const billingAccountId = await store.create('Billing Account')
    expect(billingAccountId).toBe('ba_created')

    const checkout = await store.createCheckoutSession({
      billingAccountId,
      successUrl: 'https://account.example/success',
      cancelUrl: 'https://account.example/cancel',
    })

    expect(checkout).toEqual({
      checkoutUrl: 'https://checkout.example/test',
    })
    expect(createCheckoutSession).toHaveBeenCalledWith(
      expect.objectContaining({ billingAccountId: 'ba_created' }),
      expect.any(AbortSignal),
    )
    await vi.waitFor(() =>
      expect(store.getSnapshot()).toMatchObject({
        data: { accounts: [{ id: 'ba_created' }] },
        stale: false,
        error: null,
      }),
    )
  })

  it('keeps old data stale after refresh failure and recovers explicitly', async () => {
    const refreshFailure = new Error('list unavailable')
    const listManagedBillingAccounts = vi
      .fn()
      .mockResolvedValueOnce({ accounts: [{ id: 'ba_1', displayName: 'Old' }] })
      .mockRejectedValueOnce(refreshFailure)
      .mockResolvedValueOnce({ accounts: [{ id: 'ba_1', displayName: 'New' }] })
    const renameBillingAccount = vi.fn().mockResolvedValue({})
    const store = buildStore({
      listManagedBillingAccounts,
      renameBillingAccount,
    })
    await store.refresh()

    await expect(store.rename('ba_1', 'New')).resolves.toEqual({})
    await vi.waitFor(() =>
      expect(store.getSnapshot().error).toBe(refreshFailure),
    )
    expect(store.getSnapshot()).toMatchObject({
      data: { accounts: [{ id: 'ba_1', displayName: 'Old' }] },
      loading: false,
      stale: true,
    })

    await store.refresh()

    expect(store.getSnapshot()).toMatchObject({
      data: { accounts: [{ id: 'ba_1', displayName: 'New' }] },
      loading: false,
      stale: false,
      error: null,
    })
  })

  it('does not reject a completed detach when its refresh is aborted', async () => {
    const refresh = deferred<{ accounts: [] }>()
    const listManagedBillingAccounts = vi
      .fn()
      .mockResolvedValueOnce({ accounts: [{ id: 'ba_1' }] })
      .mockImplementationOnce(() => refresh.promise)
      .mockResolvedValueOnce({ accounts: [] })
    const detachBillingAccount = vi.fn().mockResolvedValue({})
    const store = buildStore({
      detachBillingAccount,
      listManagedBillingAccounts,
    })
    const unsubscribe = store.subscribe(vi.fn())
    await vi.waitFor(() => expect(store.getSnapshot().stale).toBe(false))

    const result = store.detach('account', 'acct_1')
    await expect(result).resolves.toEqual({})
    unsubscribe()

    expect(store.getSnapshot()).toMatchObject({
      data: { accounts: [{ id: 'ba_1' }] },
      stale: true,
    })

    const unsubscribeNext = store.subscribe(vi.fn())
    await vi.waitFor(() =>
      expect(store.getSnapshot()).toMatchObject({
        data: { accounts: [] },
        stale: false,
        error: null,
      }),
    )
    unsubscribeNext()
  })

  it('clears queued work when the last consumer unsubscribes', async () => {
    const load = deferred<{ accounts: [] }>()
    const running = deferred<{}>()
    let runningSignal: AbortSignal | undefined
    const renameBillingAccount = vi.fn(
      (_id: string, _name: string, signal: AbortSignal) => {
        runningSignal = signal
        return running.promise
      },
    )
    const store = buildStore({
      listManagedBillingAccounts: vi.fn(() => load.promise),
      renameBillingAccount,
    })
    const unsubscribe = store.subscribe(vi.fn())
    const first = store.rename('ba_1', 'first')
    const queued = store.rename('ba_1', 'second')

    unsubscribe()
    expect(runningSignal?.aborted).toBe(true)
    await expect(first).rejects.toMatchObject({ name: 'AbortError' })
    await expect(queued).rejects.toMatchObject({ name: 'AbortError' })
    running.resolve({})
    load.resolve({ accounts: [] })
    await Promise.resolve()

    expect(renameBillingAccount).toHaveBeenCalledTimes(1)
    expect(store.getSnapshot().data).toBeNull()
  })
})
