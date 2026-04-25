import { lazy } from 'react'

export const DocsTypeID = 'spacewave-notes/docs'

export const DocsViewer = lazy(() => import('../../plugin/notes/DocsViewer.js'))
