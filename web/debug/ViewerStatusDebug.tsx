import { LuNotebook, LuPlus } from 'react-icons/lu'

import { useParams } from '@s4wave/web/router/router.js'
import {
  ViewerBar,
  type ViewerBarAction,
} from '@s4wave/web/object/ViewerBar.js'
import {
  ViewerStatus,
  type ViewerStatusProps,
} from '@s4wave/web/object/ViewerStatus.js'

const noop = () => {}

// add is the press an empty record offers; a viewer's presses show only once
// its record has arrived.
const add: ViewerBarAction = {
  id: 'add',
  label: 'Add note',
  icon: <LuPlus />,
  onClick: noop,
}

// fixtures are the status states the sweep opens, by the route's state
// segment.
const fixtures: Record<string, ViewerStatusProps> = {
  waiting: { state: 'waiting', what: 'the notebook' },
  failed: {
    state: 'failed',
    message: 'Could not read the notebook: the Space did not answer.',
    retry: noop,
  },
  final: {
    state: 'failed',
    message:
      'Could not read the notebook: its record is not a notebook. Reading it again cannot help.',
  },
  long: {
    state: 'failed',
    message:
      'Could not read the notebook: rpc stream closed: the peer 12D3KooWQbBvQ6GZrXv3Y1HkZ5o7LmLmyb9wUvC2Nm1oYpE6Xb2K reset the connection while reading block bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku.',
    retry: noop,
  },
  empty: {
    state: 'empty',
    message: 'No notes yet. Press Add note to write the first.',
  },
}

// ViewerStatusDebug renders a viewer's bar over ViewerStatus in one fixture
// state, with the viewport as its pane, for the viewer sweep. The route's
// state segment picks the fixture; an unknown or missing one shows waiting.
export function ViewerStatusDebug() {
  const { state: name } = useParams()
  const status = fixtures[name ?? ''] ?? fixtures.waiting
  return (
    <div className="bg-background viewer-pane flex h-full w-full flex-col">
      <h1 className="sr-only">Viewer status</h1>
      <ViewerBar
        icon={<LuNotebook />}
        title="Field notes"
        actions={status.state === 'empty' ? [add] : []}
      />
      <div className="min-h-0 flex-1">
        <ViewerStatus {...status} />
      </div>
    </div>
  )
}
