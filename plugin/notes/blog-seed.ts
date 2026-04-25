import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'
import {
  FsInitOp,
  FSType,
} from '@go/github.com/s4wave/spacewave/db/unixfs/world/unixfs.pb.js'
import { Blog } from './proto/blog.pb.js'
import { Notebook } from './proto/notebook.pb.js'
import { createObjectWithBlockData } from './object-block.js'
import { uploadSeedTree } from './unixfs-seed.js'

async function runBlogSeedStep<T>(
  label: string,
  cb: () => Promise<T>,
): Promise<T> {
  try {
    return await cb()
  } catch (err) {
    throw new Error(
      label + ': ' + (err instanceof Error ? err.message : String(err)),
    )
  }
}

async function withWritableBlogState<T>(
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

export function buildSeedBlogPost(blogName: string, date: string): string {
  return (
    '---\n' +
    'title: Hello World\n' +
    'date: ' + date + '\n' +
    'author: spacewave\n' +
    'summary: Welcome to ' + blogName + '.\n' +
    'tags: [welcome]\n' +
    'draft: false\n' +
    '---\n' +
    '\n' +
    '# Hello World\n' +
    '\n' +
    'This is your first post in ' + blogName + '.\n' +
    '\n' +
    'Edit this post from the companion notebook or switch this blog into edit mode to keep writing.\n'
  )
}

export async function ensureBlogCompanionNotebook(
  worldState: IWorldState,
  blogObjectKey: string,
  blogName: string,
  abortSignal?: AbortSignal,
): Promise<void> {
  const notebookKey = blogObjectKey + '-notebook'
  const existing = await worldState.getObject(notebookKey, abortSignal)
  if (existing) {
    existing.release()
    return
  }

  const notebook: Notebook = {
    name: blogName + ' Notes',
    sources: [{ name: 'Posts', ref: blogObjectKey + '-fs/-/' }],
  }
  await runBlogSeedStep('create companion notebook object', async () => {
    await createObjectWithBlockData(
      worldState,
      notebookKey,
      Notebook.toBinary(notebook),
      abortSignal,
    )
  })

  await runBlogSeedStep('set companion notebook type', async () => {
    await setObjectType(
      worldState,
      notebookKey,
      'spacewave-notes/notebook',
      abortSignal,
    )
  })
}

export async function createBlogClientSide(
  worldState: IWorldState,
  blogObjectKey: string,
  blogName: string,
  description: string,
  authorRegistryPath: string,
  timestamp: Date,
  abortSignal?: AbortSignal,
): Promise<void> {
  await withWritableBlogState(worldState, abortSignal, async (writeState) => {
    const unixfsKey = blogObjectKey + '-fs'

    await runBlogSeedStep('initialize blog unixfs root', async () => {
      await writeState.applyWorldOp(
        'hydra/unixfs/init',
        FsInitOp.toBinary({
          objectKey: unixfsKey,
          fsType: FSType.FSType_FS_NODE,
          timestamp,
        }),
        '',
        abortSignal,
      )
    })
    await runBlogSeedStep('seed initial blog tree', async () => {
      await uploadSeedTree(
        writeState,
        unixfsKey,
        [
          {
            path: 'hello-world.md',
            content: buildSeedBlogPost(
              blogName,
              timestamp.toISOString().slice(0, 10),
            ),
          },
        ],
        undefined,
        abortSignal,
      )
    })

    const blogData = Blog.toBinary({
      name: blogName,
      description,
      sources: [{ name: 'Posts', ref: unixfsKey + '/-/' }],
      authorRegistryPath,
    })
    await runBlogSeedStep('create blog object', async () => {
      await createObjectWithBlockData(
        writeState,
        blogObjectKey,
        blogData,
        abortSignal,
      )
    })

    await runBlogSeedStep('set blog type', async () => {
      await setObjectType(
        writeState,
        blogObjectKey,
        'spacewave-notes/blog',
        abortSignal,
      )
    })
    await ensureBlogCompanionNotebook(
      writeState,
      blogObjectKey,
      blogName,
      abortSignal,
    )
  })
}
