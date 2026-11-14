import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { Notebook } from './proto/notebook.pb.js'
import { Documentation } from './proto/docs.pb.js'
import { applySeed, seedTypedObject, seedUnixfsRoot } from './seed-steps.js'

export const NOTEBOOK_WELCOME_MARKDOWN =
  '# Welcome\n\nStart capturing notes in this Spacewave notebook.\n'

export const NOTEBOOK_GETTING_STARTED_MARKDOWN =
  '# Getting started\n\nCreate a note, organize it in folders, and keep writing locally.\n'

export const DOCS_INDEX_MARKDOWN =
  '# Documentation\n\nStart documenting this project here.\n'

export function buildNotebookUnixfsObjectKey(
  notebookObjectKey: string,
): string {
  return notebookObjectKey + '-fs'
}

// seedNotebook writes a notebook and the UnixFS root of sample notes it
// reads, as applySeed decides.
export async function seedNotebook(
  worldState: IWorldState,
  notebookObjectKey: string,
  unixfsObjectKey: string,
  notebookName: string,
  timestamp: Date,
  abortSignal?: AbortSignal,
): Promise<void> {
  await applySeed(
    worldState,
    seedTypedObject(
      'create notebook object',
      notebookObjectKey,
      Notebook.toBinary({
        name: notebookName,
        sources: [{ name: 'My Notes', ref: unixfsObjectKey + '/-/' }],
      }),
      'notes/notebook',
    ),
    [
      seedUnixfsRoot(
        'seed notes unixfs root',
        unixfsObjectKey,
        [
          { path: 'welcome.md', content: NOTEBOOK_WELCOME_MARKDOWN },
          {
            path: 'getting-started.md',
            content: NOTEBOOK_GETTING_STARTED_MARKDOWN,
          },
        ],
        timestamp,
      ),
    ],
    abortSignal,
  )
}

// seedDocs writes a Documentation object and the UnixFS root of its index
// page, as applySeed decides.
export async function seedDocs(
  worldState: IWorldState,
  docsObjectKey: string,
  docsName: string,
  description: string,
  timestamp: Date,
  abortSignal?: AbortSignal,
): Promise<void> {
  const unixfsObjectKey = docsObjectKey + '-fs'
  await applySeed(
    worldState,
    seedTypedObject(
      'create docs object',
      docsObjectKey,
      Documentation.toBinary({
        name: docsName,
        description,
        sources: [{ name: 'Pages', ref: unixfsObjectKey + '/-/' }],
        createdAt: timestamp,
      }),
      'notes/docs',
    ),
    [
      seedUnixfsRoot(
        'seed docs unixfs root',
        unixfsObjectKey,
        [{ path: 'index.md', content: DOCS_INDEX_MARKDOWN }],
        timestamp,
      ),
    ],
    abortSignal,
  )
}
