import { definePlugin } from '../../sdk/sync/plugin.js'
import { colors } from './app.js'

export default definePlugin({
  app: colors,
  displayName: 'Color votes',
  description: 'Choose a color and vote together.',
  iconName: 'palette',
  quickstart: {
    objectKey: 'colors',
    initial: { kind: 'mutate', name: 'initialize', input: null },
  },
  viewer: {
    entry: 'plugin/colors/ColorViewer.tsx',
    componentId: 'colors/votes',
  },
})
