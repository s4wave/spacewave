import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'
import { InitUnixFSOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { INIT_UNIXFS_OP_ID } from '@s4wave/core/space/world/ops/init-unixfs.js'
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
      { cause: err },
    )
  }
}

export function buildSeedBlogPost(blogName: string, date: string): string {
  return (
    '---\n' +
    'title: Hello World\n' +
    'date: ' +
    date +
    '\n' +
    'author: spacewave\n' +
    'summary: Welcome to ' +
    blogName +
    '.\n' +
    'tags: [welcome]\n' +
    'draft: false\n' +
    '---\n' +
    '\n' +
    '# Hello World\n' +
    '\n' +
    'This is your first post in ' +
    blogName +
    '.\n' +
    '\n' +
    'Edit this post from the companion notebook or switch this blog into edit mode to keep writing.\n'
  )
}

export async function ensureBlogCompanionNotebook(
  worldState: IWorldState,
  blogObjectKey: string,
  unixfsObjectKey: string,
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
    sources: [{ name: 'Posts', ref: unixfsObjectKey + '/-/' }],
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
    await setObjectType(worldState, notebookKey, 'notes/notebook', abortSignal)
  })
}

export function buildBlogUnixfsObjectKey(blogObjectKey: string): string {
  if (blogObjectKey.startsWith('blog/')) {
    return blogObjectKey.replace(/^blog\//, 'fs/blog/')
  }
  return blogObjectKey + '-fs'
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
  const unixfsKey = buildBlogUnixfsObjectKey(blogObjectKey)

  await runBlogSeedStep('initialize blog unixfs root', async () => {
    await worldState.applyWorldOp(
      INIT_UNIXFS_OP_ID,
      InitUnixFSOp.toBinary({
        objectKey: unixfsKey,
        timestamp,
      }),
      '',
      abortSignal,
    )
  })
  await runBlogSeedStep('seed initial blog tree', async () => {
    await uploadSeedTree(
      worldState,
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
  // eslint-disable-next-line react-doctor/async-parallel
  await runBlogSeedStep('create blog object', async () => {
    await createObjectWithBlockData(
      worldState,
      blogObjectKey,
      blogData,
      abortSignal,
    )
  })

  await runBlogSeedStep('set blog type', async () => {
    await setObjectType(worldState, blogObjectKey, 'notes/blog', abortSignal)
  })
  await ensureBlogCompanionNotebook(
    worldState,
    blogObjectKey,
    unixfsKey,
    blogName,
    abortSignal,
  )
}
