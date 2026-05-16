package s4wave_apt

import "github.com/aperturerobotics/cayley/quad"

// PredAptRepoPackage links an AptRepository to an AptPackage.
var PredAptRepoPackage = quad.IRI("spacewave-vm/apt/repo-package")

// PredAptRepoBuildSpec links an AptRepository to an AptBuildSpec.
var PredAptRepoBuildSpec = quad.IRI("spacewave-vm/apt/repo-buildspec")

// PredAptPackageBuildSpec links an AptPackage to the AptBuildSpec that produced it.
var PredAptPackageBuildSpec = quad.IRI("spacewave-vm/apt/package-buildspec")
