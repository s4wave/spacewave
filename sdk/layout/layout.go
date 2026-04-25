package s4wave_layout

import "github.com/pkg/errors"

// TabDefValidator validates a TabDef, particularly the data field.
type TabDefValidator = func(m *TabDef) error

// Validate checks the layout model for validity.
//
// validateTabDef can be nil.
func (m *LayoutModel) Validate(validateTabDef TabDefValidator) error {
	if m == nil {
		return nil
	}
	if err := ValidateBorders(m.GetBorders()); err != nil {
		return errors.Wrap(err, "borders")
	}
	if err := m.GetLayout().Validate(validateTabDef); err != nil {
		return err
	}
	return nil
}

// Validate checks the row def for validity.
//
// validateTabDef can be nil.
func (m *RowDef) Validate(validateTabDef TabDefValidator) error {
	if m == nil {
		return nil
	}
	for i, child := range m.GetChildren() {
		if err := child.Validate(validateTabDef); err != nil {
			return errors.Wrapf(err, "children[%d]", i)
		}
	}
	return nil
}

// Validate checks the row or tabset def for validity.
//
// validateTabDef can be nil.
func (m *RowOrTabSetDef) Validate(validateTabDef TabDefValidator) error {
	if m == nil {
		return nil
	}
	switch body := m.GetNode().(type) {
	case *RowOrTabSetDef_Row:
		if err := body.Row.Validate(validateTabDef); err != nil {
			return errors.Wrap(err, "row")
		}
	case *RowOrTabSetDef_TabSet:
		if err := body.TabSet.Validate(validateTabDef); err != nil {
			return errors.Wrap(err, "tab_set")
		}
	default:
		return errors.New("row or tabset def: must contain a row or tabset")
	}
	return nil
}

// Validate validates the TabSet.
//
// validateTabDef can be nil.
func (m *TabSetDef) Validate(validateTabDef TabDefValidator) error {
	if m == nil {
		return nil
	}
	for i, child := range m.GetChildren() {
		if err := child.Validate(validateTabDef); err != nil {
			return errors.Wrapf(err, "children[%d]", i)
		}
	}
	return nil
}

// Validate validates the tab def.
//
// validateTabDef can be nil.
func (m *TabDef) Validate(validateTabDef TabDefValidator) error {
	if m == nil {
		return nil
	}
	if len(m.GetId()) == 0 {
		return errors.New("tab_def: id cannot be empty")
	}
	if validateTabDef != nil {
		if err := validateTabDef(m); err != nil {
			return err
		}
	}
	return nil
}

// ValidateBorders checks if the borders list has any duplicates.
func ValidateBorders(borders []*BorderDef) error {
	var seen byte
	for _, border := range borders {
		loc := border.GetBorderLocation()
		if err := loc.Validate(); err != nil {
			return err
		}
		if seen&(1<<(byte(loc)-1)) != 0 {
			return errors.Errorf("duplicate border side: %v", loc.String())
		}
		seen |= (1 << (byte(loc) - 1))
	}
	return nil
}

// Validate validates the border location.
func (b BorderLocation) Validate() error {
	switch b {
	case BorderLocation_BorderLocation_LEFT:
	case BorderLocation_BorderLocation_BOTTOM:
	case BorderLocation_BorderLocation_RIGHT:
	case BorderLocation_BorderLocation_TOP:
	default:
		return errors.Errorf("invalid border location: %v", b.String())
	}
	return nil
}
