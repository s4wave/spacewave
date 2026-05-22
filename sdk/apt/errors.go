package s4wave_apt

import "errors"

var (
	// ErrInvalidDebPackage is returned when a .deb package cannot be parsed.
	ErrInvalidDebPackage = errors.New("apt package: invalid deb package")
	// ErrUnsupportedDebControlCompression is returned for unsupported control archive compression.
	ErrUnsupportedDebControlCompression = errors.New("apt package: unsupported control archive compression")
	// ErrUnknownAptRepositoryState is returned for an unknown AptRepository state.
	ErrUnknownAptRepositoryState = errors.New("apt repository: unknown state")
	// ErrInvalidAptRepositoryStateTransition is returned for an illegal AptRepository state transition.
	ErrInvalidAptRepositoryStateTransition = errors.New("apt repository: invalid state transition")
	// ErrInvalidAptRepositoryInitialState is returned when creating a repository outside its seed state.
	ErrInvalidAptRepositoryInitialState = errors.New("apt repository: invalid initial state")
	// ErrUnknownAptPackageState is returned for an unknown AptPackage state.
	ErrUnknownAptPackageState = errors.New("apt package: unknown state")
	// ErrInvalidAptPackageStateTransition is returned for an illegal AptPackage state transition.
	ErrInvalidAptPackageStateTransition = errors.New("apt package: invalid state transition")
	// ErrInvalidAptPackageInitialState is returned when creating a package outside its seed states.
	ErrInvalidAptPackageInitialState = errors.New("apt package: invalid initial state")
	// ErrInvalidAptPackageIndexMetadata is returned when a package cannot be rendered into an apt index.
	ErrInvalidAptPackageIndexMetadata = errors.New("apt package: invalid index metadata")
)
