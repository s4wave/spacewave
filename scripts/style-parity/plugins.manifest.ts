import routes from './routes.manifest.ts'

// pluginsManifest checks the exact plugin connections before repeating the full interface matrix.
const pluginsManifest = {
  states: routes.states.filter((state) =>
    ['viewer-notebook', 'viewer-sql', 'viewer-docs', 'viewer-blog'].includes(
      state.id,
    ),
  ),
}

export default pluginsManifest
