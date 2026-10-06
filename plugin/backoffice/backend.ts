import { definePlugin } from '../../sdk/sync/plugin.js'
import { backoffice } from './app.js'

export default definePlugin({
  app: backoffice,
  displayName: 'Spacewave backoffice',
  description: 'Read Spacewave accounts, billing and storage.',
  iconName: 'layout',
  quickstart: { objectKey: 'backoffice' },
  viewer: {
    entry: 'plugin/backoffice/BackofficeViewer.tsx',
    componentId: 'spacewave/backoffice',
  },
})
