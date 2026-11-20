package s4wave_wizard

import "github.com/s4wave/spacewave/db/world"

// WorldInternal marks the wizard endings object internal: it holds
// bookkeeping, not anything a person made.
func (*WizardEndings) WorldInternal() {}

// _ is a type assertion
var _ world.InternalBlock = (*WizardEndings)(nil)
