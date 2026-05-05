package s4wave_wizard

import "github.com/pkg/errors"

var (
	// ErrWizardRequired is returned when RegisterWizard has no wizard body.
	ErrWizardRequired = errors.New("wizard registration is required")
	// ErrWizardTypeIDRequired is returned when a wizard registration has no type id.
	ErrWizardTypeIDRequired = errors.New("wizard type id is required")
	// ErrWizardPluginIDRequired is returned when a wizard registration has no plugin id.
	ErrWizardPluginIDRequired = errors.New("wizard plugin id is required")
	// ErrWizardNameRequired is returned when a wizard registration has no display name.
	ErrWizardNameRequired = errors.New("wizard display name is required")
	// ErrWizardAlreadyRegistered is returned when a dynamic wizard type id is already registered.
	ErrWizardAlreadyRegistered = errors.New("wizard type id is already registered")
)
