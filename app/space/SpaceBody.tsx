import { Route, Routes } from '@s4wave/web/router/router.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'

import { SpaceObjectContainer } from './SpaceObjectContainer.js'
import { SpaceDebug } from './SpaceDebug.js'
import { SpaceIndex } from './SpaceIndex.js'

export function SpaceBody() {
  return (
    <Routes>
      <Route path="/">
        <SpaceIndex />
      </Route>
      <Route path="/debug">
        <SpaceDebug />
      </Route>
      <Route path="/*">
        <SpaceObjectContainer />
      </Route>
      <Route path="*">
        <NavigatePath to="../" replace />
      </Route>
    </Routes>
  )
}
