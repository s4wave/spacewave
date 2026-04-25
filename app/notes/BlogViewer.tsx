import { lazy } from 'react'

export const BlogTypeID = 'spacewave-notes/blog'

export const BlogViewer = lazy(() => import('../../plugin/notes/BlogViewer.js'))
