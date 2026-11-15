import type { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'
import { FSNodeTypeID } from '@go/github.com/s4wave/spacewave/db/unixfs/block/fstree_typeid.pb.js'
import type { WorldStage } from '@s4wave/sdk/world/stage.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import {
  buildStagedTree,
  type StagedTreeFile,
} from '@s4wave/sdk/unixfs/world/stage.js'
import {
  DOCS_INDEX_MARKDOWN,
  NOTEBOOK_GETTING_STARTED_MARKDOWN,
  NOTEBOOK_WELCOME_MARKDOWN,
} from './content-seed.js'
import { buildBlogUnixfsObjectKey, buildSeedBlogPost } from './blog-seed.js'
import { stageObjectBlock } from './object-block.js'
import { applySeed, seedStagedObject } from './seed-steps.js'
import { Blog, CreateBlogStagedOp } from './proto/blog.pb.js'
import { CreateDocumentationStagedOp, Documentation } from './proto/docs.pb.js'
import { InitNotebookStagedOp, Notebook } from './proto/notebook.pb.js'
import { INIT_NOTEBOOK_STAGED_OP_ID } from './proto/init-notebook.js'
import { CREATE_BLOG_STAGED_OP_ID } from './proto/create-blog.js'
import { CREATE_DOCS_STAGED_OP_ID } from './proto/create-docs.js'

// A staged notes create runs in two halves. The sender builds every object's
// content in a World stage it opened, outside any World transaction, and
// sends the op naming the staged roots. The op handler, under the World
// writer, only reads the keys and adopts the roots as applySeed decides. The
// sender holds the stage until the op's transaction returns.

// StagedNotesCreate is a staged create op ready to send.
export interface StagedNotesCreate {
  opTypeId: string
  opData: Uint8Array
}

// stageNotebookCreate builds a notebook named name and its starter notes in
// stage and returns the op that creates them at notebookKey and unixfsKey.
export async function stageNotebookCreate(
  stage: WorldStage,
  notebookKey: string,
  unixfsKey: string,
  name: string,
  abortSignal?: AbortSignal,
): Promise<StagedNotesCreate> {
  const [notebookRoot, unixfsRoot] = await Promise.all([
    stageBlock(
      stage,
      Notebook.toBinary({
        name,
        sources: [{ name: 'My Notes', ref: unixfsKey + '/-/' }],
      }),
      abortSignal,
    ),
    buildStagedTree(
      stage,
      [
        textFile('welcome.md', NOTEBOOK_WELCOME_MARKDOWN),
        textFile('getting-started.md', NOTEBOOK_GETTING_STARTED_MARKDOWN),
      ],
      abortSignal,
    ),
  ])
  return {
    opTypeId: INIT_NOTEBOOK_STAGED_OP_ID,
    opData: InitNotebookStagedOp.toBinary({
      notebookObjectKey: notebookKey,
      unixfsObjectKey: unixfsKey,
      notebookRoot,
      unixfsRoot,
    }),
  }
}

// stageDocsCreate builds a Documentation named name, created at timestamp,
// and its index page in stage, and returns the op that creates them at
// docsKey.
export async function stageDocsCreate(
  stage: WorldStage,
  docsKey: string,
  name: string,
  timestamp: Date,
  abortSignal?: AbortSignal,
): Promise<StagedNotesCreate> {
  const [docsRoot, unixfsRoot] = await Promise.all([
    stageBlock(
      stage,
      Documentation.toBinary({
        name,
        description: '',
        sources: [{ name: 'Pages', ref: docsUnixfsKey(docsKey) + '/-/' }],
        createdAt: timestamp,
      }),
      abortSignal,
    ),
    buildStagedTree(
      stage,
      [textFile('index.md', DOCS_INDEX_MARKDOWN)],
      abortSignal,
    ),
  ])
  return {
    opTypeId: CREATE_DOCS_STAGED_OP_ID,
    opData: CreateDocumentationStagedOp.toBinary({
      objectKey: docsKey,
      docsRoot,
      unixfsRoot,
    }),
  }
}

// stageBlogCreate builds a blog named name, its first post dated timestamp,
// and its companion notebook in stage, and returns the op that creates them
// at blogKey.
export async function stageBlogCreate(
  stage: WorldStage,
  blogKey: string,
  name: string,
  timestamp: Date,
  abortSignal?: AbortSignal,
): Promise<StagedNotesCreate> {
  const sources = [
    { name: 'Posts', ref: buildBlogUnixfsObjectKey(blogKey) + '/-/' },
  ]
  const post = buildSeedBlogPost(name, timestamp.toISOString().slice(0, 10))
  const [blogRoot, unixfsRoot, notebookRoot] = await Promise.all([
    stageBlock(
      stage,
      Blog.toBinary({ name, description: '', sources, authorRegistryPath: '' }),
      abortSignal,
    ),
    buildStagedTree(stage, [textFile('hello-world.md', post)], abortSignal),
    stageBlock(
      stage,
      Notebook.toBinary({ name: name + ' Notes', sources }),
      abortSignal,
    ),
  ])
  return {
    opTypeId: CREATE_BLOG_STAGED_OP_ID,
    opData: CreateBlogStagedOp.toBinary({
      objectKey: blogKey,
      blogRoot,
      unixfsRoot,
      notebookRoot,
    }),
  }
}

// applyNotebookStaged creates the notebook and its UnixFS root from op's
// staged roots, as applySeed decides.
export async function applyNotebookStaged(
  worldState: IWorldState,
  op: InitNotebookStagedOp,
  abortSignal?: AbortSignal,
): Promise<void> {
  const notebookKey = op.notebookObjectKey ?? ''
  const unixfsKey = op.unixfsObjectKey ?? ''
  if (!notebookKey || !unixfsKey) {
    throw new Error('notebookObjectKey and unixfsObjectKey are required')
  }
  await applySeed(
    worldState,
    seedStagedObject(
      'create notebook object',
      notebookKey,
      'notes/notebook',
      op.notebookRoot,
    ),
    [
      seedStagedObject(
        'create notes unixfs root',
        unixfsKey,
        FSNodeTypeID,
        op.unixfsRoot,
      ),
    ],
    abortSignal,
  )
}

// applyDocsStaged creates the Documentation and its UnixFS root from op's
// staged roots, as applySeed decides.
export async function applyDocsStaged(
  worldState: IWorldState,
  op: CreateDocumentationStagedOp,
  abortSignal?: AbortSignal,
): Promise<void> {
  const docsKey = op.objectKey ?? ''
  if (!docsKey) throw new Error('objectKey is required')
  await applySeed(
    worldState,
    seedStagedObject('create docs object', docsKey, 'notes/docs', op.docsRoot),
    [
      seedStagedObject(
        'create docs unixfs root',
        docsUnixfsKey(docsKey),
        FSNodeTypeID,
        op.unixfsRoot,
      ),
    ],
    abortSignal,
  )
}

// applyBlogStaged creates the blog, its UnixFS root and its companion
// notebook from op's staged roots, as applySeed decides.
export async function applyBlogStaged(
  worldState: IWorldState,
  op: CreateBlogStagedOp,
  abortSignal?: AbortSignal,
): Promise<void> {
  const blogKey = op.objectKey ?? ''
  if (!blogKey) throw new Error('objectKey is required')
  await applySeed(
    worldState,
    seedStagedObject('create blog object', blogKey, 'notes/blog', op.blogRoot),
    [
      seedStagedObject(
        'create blog unixfs root',
        buildBlogUnixfsObjectKey(blogKey),
        FSNodeTypeID,
        op.unixfsRoot,
      ),
      seedStagedObject(
        'create companion notebook object',
        blogKey + '-notebook',
        'notes/notebook',
        op.notebookRoot,
      ),
    ],
    abortSignal,
  )
}

// docsUnixfsKey is the key of a Documentation's UnixFS root.
function docsUnixfsKey(docsKey: string): string {
  return docsKey + '-fs'
}

// stageBlock writes data as one block in stage and returns the object root
// naming it.
async function stageBlock(
  stage: WorldStage,
  data: Uint8Array,
  abortSignal?: AbortSignal,
): Promise<ObjectRef> {
  using cursor = await stage.buildStorageCursor(abortSignal)
  return stageObjectBlock(cursor, data, abortSignal)
}

// textFile is a staged tree file holding text.
function textFile(path: string, text: string): StagedTreeFile {
  return { path, data: new TextEncoder().encode(text) }
}
