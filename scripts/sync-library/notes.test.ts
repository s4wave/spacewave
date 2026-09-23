import assert from 'node:assert/strict'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'

import { Application } from '../../core/sync/application.js'
import { openNodeEngine } from '../../core/sync/node/host.js'
import { watchObjectQuery } from '../../sdk/sync/object-query.js'
import { accessObjectRootWorldState } from '../../sdk/world/utils.js'
import {
  createObjectWithBlockData,
  setObjectBlockData,
} from '../../plugin/notes/object-block.js'
import { Notebook } from '../../plugin/notes/proto/notebook.pb.js'
import {
  notebookViews,
  notebookSavedViews,
  type SavedView,
} from '../../plugin/notes/saved-views.js'

test(
  'Notes views synchronize between clients, persist, and leave canonical Notebook data intact',
  { timeout: 60_000 },
  async () => {
    // Run the production typed app and object source against the compiled World engine.
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-notes-'))
    let owner = await openNodeEngine(directory)
    const buildApp = () =>
      new Application({
        engine: owner.engine,
        ...notebookViews,
        instance: 'notebook/saved-views',
        authorize: () => true,
      })
    let app = buildApp()
    const controller = new AbortController()
    let notebookReads = 0
    const notebook = {
      name: 'Notes',
      sources: [{ name: 'Files', ref: 'files/-/' }],
    }
    const bytes = Notebook.toBinary(notebook)
    const tx = await owner.engine.newTransaction(true)
    try {
      await createObjectWithBlockData(tx, 'notebook', bytes)
      await tx.commit()
    } finally {
      await tx.discard()
      tx.release()
    }
    const source = watchObjectQuery(
      owner.engine,
      'notebook',
      async (object, signal) => {
        using cursor = await accessObjectRootWorldState(object, signal)
        const block = await cursor.getBlock({}, signal)
        notebookReads++
        return Notebook.fromBinary(block.data!)
      },
      controller.signal,
    )[Symbol.asyncIterator]()
    await app.initialize()
    const alice = app.access({ subject: 'alice', scope: 'shared' })
    const bob = app.access({ subject: 'bob', scope: 'shared' })
    const a = alice
      .watch(notebookSavedViews, { notebook: 'notebook' }, controller.signal)
      [Symbol.asyncIterator]()
    const b = bob
      .watch(notebookSavedViews, { notebook: 'notebook' }, controller.signal)
      [Symbol.asyncIterator]()
    try {
      assert.deepEqual(
        Notebook.toBinary((await source.next()).value!.value!),
        bytes,
      )
      assert.deepEqual((await a.next()).value?.value, [])
      assert.deepEqual((await b.next()).value?.value, [])
      const nextNotebook = source.next()
      void nextNotebook.catch(() => {})
      const view: SavedView = {
        id: 'review',
        notebook: 'notebook',
        name: 'Review',
        sourceRef: 'files/-/',
        path: 'team',
        filterTag: 'review',
        filterStatus: 'TODO',
        sort: 'title',
      }

      // Named operations update both principal-bound subscriptions without reopening them.
      await alice.mutate('saveView', view, { requestId: 'save' })
      assert.deepEqual((await a.next()).value?.value, [view])
      assert.deepEqual((await b.next()).value?.value, [view])
      await bob.mutate(
        'renameView',
        { id: view.id, name: 'Weekly review' },
        { requestId: 'rename' },
      )
      const renamed = { ...view, name: 'Weekly review' }
      assert.deepEqual((await a.next()).value?.value, [renamed])
      assert.deepEqual((await b.next()).value?.value, [renamed])

      // A later Notebook edit proves unrelated saved-view commits caused no block reads.
      const update = await owner.engine.newTransaction(true)
      try {
        using object = await update.getObject('notebook')
        await setObjectBlockData(
          object!,
          Notebook.toBinary({ ...notebook, name: 'Renamed Notebook' }),
        )
        await update.commit()
      } finally {
        await update.discard()
        update.release()
      }
      assert.equal((await nextNotebook).value?.value?.name, 'Renamed Notebook')
      assert.equal(notebookReads, 2)

      // Closing all attachments and reopening retains the additive data and original sources.
      controller.abort()
      await Promise.allSettled([source.return?.(), a.return?.(), b.return?.()])
      await app.close()
      await owner.close()
      owner = await openNodeEngine(directory)
      app = buildApp()
      await app.initialize()
      const reopened = app.access({ subject: 'alice', scope: 'shared' })
      assert.deepEqual(
        await reopened.collection('savedViews').get(view.id),
        renamed,
      )
      const read = await owner.engine.newTransaction(false)
      try {
        using object = await read.getObject('notebook')
        using cursor = await accessObjectRootWorldState(object!)
        assert.deepEqual(
          (await cursor.getBlock({})).data,
          Notebook.toBinary({ ...notebook, name: 'Renamed Notebook' }),
        )
      } finally {
        await read.discard()
        read.release()
      }
      await reopened.mutate(
        'deleteView',
        { id: view.id },
        { requestId: 'delete' },
      )
      assert.equal(
        await reopened.collection('savedViews').get(view.id),
        undefined,
      )
    } finally {
      controller.abort()
      await Promise.allSettled([source.return?.(), a.return?.(), b.return?.()])
      await app.close()
      await owner.close()
      await rm(directory, { recursive: true, force: true })
    }
  },
)
