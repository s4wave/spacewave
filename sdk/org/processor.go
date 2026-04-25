package s4wave_org

import (
	"github.com/s4wave/spacewave/core/sobject"
	sobject_stateproc "github.com/s4wave/spacewave/core/sobject/stateproc"
)

// ProcessOrgOps is a ProcessOpsFunc that applies OrgSOOp operations to
// OrgState data. Used by the lightweight state processor for org SOs.
var ProcessOrgOps sobject.ProcessOpsFunc = sobject_stateproc.BuildProcessOpsFunc(ApplyOrgSOOp)
