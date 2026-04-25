import { lazy } from 'react'

export const NotebookTypeID = 'spacewave-notes/notebook'

export const NotebookViewer = lazy(
  () => import('../../plugin/notes/NotebookViewer.js'),
)
