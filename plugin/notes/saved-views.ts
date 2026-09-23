import { z } from 'zod'

import { defineApp } from '@s4wave/sdk/sync/app.js'
import { defineQuery } from '@s4wave/sdk/sync/query.js'
import { SyncError } from '@s4wave/sdk/sync/errors.js'

const viewName = z.string().trim().min(1).max(80)

/** savedViewSchema describes explicitly shared filters without storing note contents. */
export const savedViewSchema = z.object({
  id: z.string().min(1),
  notebook: z.string().min(1),
  name: viewName,
  sourceRef: z.string().min(1),
  path: z.string(),
  filterTag: z.string().nullable(),
  filterStatus: z.string().nullable(),
  sort: z.enum(['name', 'title']),
})

/** SavedView is a shared Notebook view definition; selecting or editing it stays personal. */
export type SavedView = z.infer<typeof savedViewSchema>

/** notebookViewsKey locates a Notebook's additive application dataset. */
export function notebookViewsKey(notebookKey: string): string {
  return `${notebookKey}/saved-views`
}

/** notebookViews owns named saved-view operations in the Notebook's World. */
export const notebookViews = defineApp(
  {
    id: 'notes/saved-views',
    version: 1,
    collections: { savedViews: savedViewSchema },
    mutations: {
      saveView: { input: savedViewSchema, output: savedViewSchema },
      renameView: {
        input: z.object({ id: z.string().min(1), name: viewName }),
        output: savedViewSchema,
      },
      deleteView: {
        input: z.object({ id: z.string().min(1) }),
        output: z.null(),
      },
    },
  },
  {
    async saveView({ collection }, view) {
      await collection('savedViews').put(view.id, view)
      return view
    },
    async renameView({ collection }, { id, name }) {
      // Read and rename within one transaction so filters are never overwritten.
      const views = collection('savedViews')
      const current = await views.get(id)
      if (!current)
        throw new SyncError('VALIDATION', 'This saved view was removed')

      // Preserve the shared configuration while changing only its name.
      const renamed = { ...current, name }
      await views.put(id, renamed)
      return renamed
    },
    async deleteView({ collection }, { id }) {
      await collection('savedViews').delete(id)
      return null
    },
  },
)

/** notebookSavedViews derives membership and name ordering from the same function query. */
export const notebookSavedViews = defineQuery(notebookViews.schema, {
  name: 'notebook-saved-views',
  input: z.object({ notebook: z.string() }),
  async evaluate({ collection }, { notebook }) {
    const rows = await collection('savedViews').scan()
    return rows
      .flatMap(({ value }) => (value.notebook === notebook ? [value] : []))
      .toSorted(
        (a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id),
      )
  },
})
