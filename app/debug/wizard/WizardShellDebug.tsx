import { LuGitBranch } from 'react-icons/lu'

import {
  WizardShell,
  type WizardShellProps,
} from '@s4wave/app/wizard/WizardShell.js'
import { useParams } from '@s4wave/web/router/router.js'

const noop = () => {}

// base is the wizard every fixture starts from: a two-step Git repository
// wizard on its first step, named.
const base: WizardShellProps = {
  icon: <LuGitBranch />,
  title: 'New Git Repository',
  step: 0,
  totalSteps: 2,
  stepName: 'Source',
  localName: 'spacewave',
  nameHelp: 'The name the Space lists the repository under.',
  onUpdateName: noop,
  onBack: noop,
  onCancel: noop,
  creating: false,
  onFinalize: noop,
  finalizeStep: 1,
  onNext: noop,
  canNext: true,
}

// fixtures are the wizard states the sweep opens, by the route's state
// segment.
const fixtures: Record<string, Partial<WizardShellProps>> = {
  name: {},
  invalid: { localName: '', nameError: 'Name the repository to continue.' },
  final: { step: 1, stepName: 'Review', canBack: true },
  creating: { step: 1, stepName: 'Review', canBack: true, creating: true },
}

// WizardShellDebug renders WizardShell in one fixture state, with the viewport
// as its pane, for the viewer sweep. The route's state segment picks the
// fixture; an unknown or missing one shows the named first step.
export function WizardShellDebug() {
  const { state: name } = useParams()
  const fixture = fixtures[name ?? ''] ?? fixtures.name
  return (
    <div className="bg-background viewer-pane h-full w-full">
      <h1 className="sr-only">Wizard</h1>
      <WizardShell {...base} {...fixture} />
    </div>
  )
}
