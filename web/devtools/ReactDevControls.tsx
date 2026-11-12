import { Button } from '@s4wave/web/ui/button.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import type {
  ReactDevControl,
  ReactDevModel,
  ReactDevView,
} from './react-dev.js'

// ReactDevAction is one button the setting offers.
type ReactDevAction = 'enable' | 'disable' | 'retry' | 'update' | 'reload'

// actionsFor lists the buttons each view shows, as ReactDev.tla's Shown does.
function actionsFor(view: ReactDevView, m: ReactDevModel): ReactDevAction[] {
  switch (view) {
    case 'off':
      return m.running ? ['enable', 'reload'] : ['enable']
    case 'fetching':
      return ['disable']
    case 'ready':
      return ['reload', 'disable']
    case 'on':
      return ['disable']
    case 'absent':
      return ['retry', 'disable']
    case 'mismatch':
      return ['update', 'disable']
  }
}

// statusFor describes what the view means for this document.
function statusFor(view: ReactDevView, m: ReactDevModel): string {
  switch (view) {
    case 'off':
      return m.running
        ? 'Turned off. Reload to return to production React.'
        : 'Readable errors and component stacks, at the cost of speed.'
    case 'fetching':
      return 'Loading development React'
    case 'ready':
      return 'Reload to start development React.'
    case 'on':
      return `Running development React ${m.release}.`
    case 'absent':
      return 'Development React could not be loaded. Running production React.'
    case 'mismatch':
      return `Saved for React ${m.cached}; this release uses ${m.release}. Running production React.`
  }
}

const labels: Record<ReactDevAction, string> = {
  enable: 'Turn on',
  disable: 'Turn off',
  retry: 'Retry',
  update: 'Update',
  reload: 'Reload',
}

// ReactDevControls shows the Development React setting's state and the
// actions its state allows.
export function ReactDevControls({ control }: { control: ReactDevControl }) {
  const { model, view, dispatch, reload } = control
  const run = (action: ReactDevAction) => {
    switch (action) {
      case 'enable':
        return dispatch({ type: 'enable' })
      case 'disable':
        return dispatch({ type: 'disable' })
      case 'retry':
      case 'update':
        return dispatch({ type: 'refetch' })
      case 'reload':
        return reload()
    }
  }
  const actions = actionsFor(view, model)
  return (
    <div className="space-y-2" data-testid="react-dev-controls">
      <div className="text-sm font-medium">Development React</div>
      <div
        className="text-foreground-alt/70 flex items-center gap-2 text-xs"
        role="status"
      >
        {view === 'fetching' && <Spinner size="sm" />}
        {statusFor(view, model)}
      </div>
      <div className="flex flex-wrap gap-2">
        {actions.map((action, i) => (
          <Button
            key={action}
            type="button"
            size="sm"
            variant={i === 0 ? 'outline' : 'ghost'}
            onClick={() => run(action)}
          >
            {labels[action]}
          </Button>
        ))}
      </div>
    </div>
  )
}
