import routes from './routes.manifest.ts'

// sessionReady checks the two session transitions before repeating the complete matrix.
export default {
  states: routes.states.filter((state) =>
    ['viewer-git', 'session-billing-unavailable'].includes(state.id),
  ),
}
