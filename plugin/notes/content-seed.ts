import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'
import { InitUnixFSOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { INIT_UNIXFS_OP_ID } from '@s4wave/core/space/world/ops/init-unixfs.js'
import { Notebook } from './proto/notebook.pb.js'
import { Documentation } from './proto/docs.pb.js'
import { createObjectWithBlockData } from './object-block.js'
import { uploadSeedTree } from './unixfs-seed.js'

export const NOTEBOOK_WELCOME_MARKDOWN =
  '# Welcome\n\nStart capturing notes in this Spacewave notebook.\n'

export const NOTEBOOK_GETTING_STARTED_MARKDOWN =
  '# Getting started\n\nCreate a note, organize it in folders, and keep writing locally.\n'

export const DOCS_INDEX_MARKDOWN =
  '# Documentation\n\nStart documenting this project here.\n'

function runContentSeedStep<T>(
  label: string,
  cb: () => Promise<T>,
): Promise<T> {
  return cb().catch((err) => {
    throw new Error(
      label + ': ' + (err instanceof Error ? err.message : String(err)),
    )
  })
}

async function withWritableNotesState<T>(
  worldState: IWorldState,
  abortSignal: AbortSignal | undefined,
  cb: (writeState: IWorldState) => Promise<T>,
): Promise<T> {
  if (!(worldState instanceof EngineWorldState)) {
    return cb(worldState)
  }

  const engine = worldState.getEngine()
  let committed = false
  const tx = await engine.newTransaction(true, abortSignal)
  try {
    const result = await cb(tx)
    await tx.commit(abortSignal)
    committed = true
    return result
  } finally {
    if (!committed) {
      await tx.discard(abortSignal).catch(() => {})
    }
    tx.release()
  }
}

export function buildNotebookUnixfsObjectKey(
  notebookObjectKey: string,
): string {
  return notebookObjectKey + '-fs'
}

async function initNotesUnixfs(
  worldState: IWorldState,
  unixfsObjectKey: string,
  timestamp: Date,
  abortSignal?: AbortSignal,
): Promise<void> {
  await runContentSeedStep('initialize notes unixfs root', async () => {
    await worldState.applyWorldOp(
      INIT_UNIXFS_OP_ID,
      InitUnixFSOp.toBinary({
        objectKey: unixfsObjectKey,
        timestamp,
      }),
      '',
      abortSignal,
    )
  })
}

export async function createNotebookClientSide(
  worldState: IWorldState,
  notebookObjectKey: string,
  unixfsObjectKey: string,
  notebookName: string,
  timestamp: Date,
  abortSignal?: AbortSignal,
): Promise<void> {
  await withWritableNotesState(worldState, abortSignal, async (writeState) => {
    // eslint-disable-next-line react-doctor/async-parallel
    await initNotesUnixfs(writeState, unixfsObjectKey, timestamp, abortSignal)
    await runContentSeedStep('seed initial notebook tree', async () => {
      await uploadSeedTree(
        writeState,
        unixfsObjectKey,
        [
          { path: 'welcome.md', content: NOTEBOOK_WELCOME_MARKDOWN },
          {
            path: 'getting-started.md',
            content: NOTEBOOK_GETTING_STARTED_MARKDOWN,
          },
        ],
        undefined,
        abortSignal,
      )
    })

    await runContentSeedStep('create notebook object', async () => {
      await createObjectWithBlockData(
        writeState,
        notebookObjectKey,
        Notebook.toBinary({
          name: notebookName,
          sources: [{ name: 'My Notes', ref: unixfsObjectKey + '/-/' }],
        }),
        abortSignal,
      )
    })

    await runContentSeedStep('set notebook type', async () => {
      await setObjectType(
        writeState,
        notebookObjectKey,
        'notes/notebook',
        abortSignal,
      )
    })
  })
}

export async function createDocsClientSide(
  worldState: IWorldState,
  docsObjectKey: string,
  docsName: string,
  description: string,
  timestamp: Date,
  abortSignal?: AbortSignal,
): Promise<void> {
  const unixfsObjectKey = docsObjectKey + '-fs'
  await withWritableNotesState(worldState, abortSignal, async (writeState) => {
    // eslint-disable-next-line react-doctor/async-parallel
    await initNotesUnixfs(writeState, unixfsObjectKey, timestamp, abortSignal)
    await runContentSeedStep('seed initial docs tree', async () => {
      await uploadSeedTree(
        writeState,
        unixfsObjectKey,
        [{ path: 'index.md', content: DOCS_INDEX_MARKDOWN }],
        undefined,
        abortSignal,
      )
    })

    await runContentSeedStep('create docs object', async () => {
      await createObjectWithBlockData(
        writeState,
        docsObjectKey,
        Documentation.toBinary({
          name: docsName,
          description,
          sources: [{ name: 'Pages', ref: unixfsObjectKey + '/-/' }],
          createdAt: timestamp,
        }),
        abortSignal,
      )
    })

    await runContentSeedStep('set docs type', async () => {
      await setObjectType(writeState, docsObjectKey, 'notes/docs', abortSignal)
    })
  })
}
