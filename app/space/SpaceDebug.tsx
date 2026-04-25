import { SpaceContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'

// SpaceDebug displays the space dashboard / home page.
export function SpaceDebug() {
  const { spaceId, spaceState, objectKey } = SpaceContainerContext.useContext()
  const spaceResource = SpaceContext.useContext()

  return (
    <div>
      <h1>Space Dashboard</h1>
      <p>Space ID: {spaceId}</p>
      <p>Object Key: {objectKey || 'None'}</p>
      <p>Ready: {spaceState.ready ? 'Yes' : 'No'}</p>
      <p>
        Space Resource: {spaceResource.loading ? 'Loading' : 'Loaded'} (Error:{' '}
        {spaceResource.error ? 'Yes' : 'No'})
      </p>
      <p>
        World Contents:{' '}
        {spaceState.worldContents ?
          JSON.stringify(spaceState.worldContents)
        : 'None'}
      </p>
    </div>
  )
}
