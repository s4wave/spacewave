package desktop_tray

// DesktopTrayEntryKind describes projected tray row semantics.
type DesktopTrayEntryKind int32

const (
	DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_UNSPECIFIED DesktopTrayEntryKind = 0
	DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_SECTION     DesktopTrayEntryKind = 1
	DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_SEPARATOR   DesktopTrayEntryKind = 2
	DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_STATUS      DesktopTrayEntryKind = 3
	DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_ACTION      DesktopTrayEntryKind = 4
	DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_SUBMENU     DesktopTrayEntryKind = 5
)

// DesktopTrayActionKind describes selectable action semantics.
type DesktopTrayActionKind int32

const (
	DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_UNSPECIFIED      DesktopTrayActionKind = 0
	DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_OPEN_ROUTE       DesktopTrayActionKind = 1
	DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_NEW_WINDOW       DesktopTrayActionKind = 2
	DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_COPY_TEXT        DesktopTrayActionKind = 3
	DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_REVEAL_PATH      DesktopTrayActionKind = 4
	DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_ATTACHED_HANDLER DesktopTrayActionKind = 5
	DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_QUIT             DesktopTrayActionKind = 6
)

// DesktopTrayIconState describes projected tray icon state.
type DesktopTrayIconState int32

const (
	DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_UNSPECIFIED  DesktopTrayIconState = 0
	DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_NORMAL       DesktopTrayIconState = 1
	DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_ACTIVE       DesktopTrayIconState = 2
	DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_ATTENTION    DesktopTrayIconState = 3
	DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_DISCONNECTED DesktopTrayIconState = 4
	DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_QUITTING     DesktopTrayIconState = 5
)

// DesktopTraySeverity describes projected tray row severity.
type DesktopTraySeverity int32

const (
	DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_UNSPECIFIED DesktopTraySeverity = 0
	DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_INFO        DesktopTraySeverity = 1
	DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_WARNING     DesktopTraySeverity = 2
	DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_CRITICAL    DesktopTraySeverity = 3
)

// DesktopTrayAction describes one projected tray action.
type DesktopTrayAction struct {
	Kind  DesktopTrayActionKind
	Route string
	Value string
}

// GetKind returns the action kind.
func (x *DesktopTrayAction) GetKind() DesktopTrayActionKind {
	if x == nil {
		return DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_UNSPECIFIED
	}
	return x.Kind
}

// GetRoute returns the route.
func (x *DesktopTrayAction) GetRoute() string {
	if x == nil {
		return ""
	}
	return x.Route
}

// GetValue returns the action value.
func (x *DesktopTrayAction) GetValue() string {
	if x == nil {
		return ""
	}
	return x.Value
}

// DesktopTrayEntry describes one projected tray row.
type DesktopTrayEntry struct {
	Id        string
	Kind      DesktopTrayEntryKind
	Label     string
	Active    bool
	Enabled   bool
	Action    *DesktopTrayAction
	Order     int32
	IconState DesktopTrayIconState
	Severity  DesktopTraySeverity
}

// GetId returns the row id.
func (x *DesktopTrayEntry) GetId() string {
	if x == nil {
		return ""
	}
	return x.Id
}

// GetKind returns the row kind.
func (x *DesktopTrayEntry) GetKind() DesktopTrayEntryKind {
	if x == nil {
		return DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_UNSPECIFIED
	}
	return x.Kind
}

// GetLabel returns the row label.
func (x *DesktopTrayEntry) GetLabel() string {
	if x == nil {
		return ""
	}
	return x.Label
}

// GetActive returns whether the row is active.
func (x *DesktopTrayEntry) GetActive() bool {
	if x == nil {
		return false
	}
	return x.Active
}

// GetEnabled returns whether the row is enabled.
func (x *DesktopTrayEntry) GetEnabled() bool {
	if x == nil {
		return false
	}
	return x.Enabled
}

// GetAction returns the row action.
func (x *DesktopTrayEntry) GetAction() *DesktopTrayAction {
	if x == nil {
		return nil
	}
	return x.Action
}

// GetOrder returns the row order.
func (x *DesktopTrayEntry) GetOrder() int32 {
	if x == nil {
		return 0
	}
	return x.Order
}

// GetIconState returns the row icon state.
func (x *DesktopTrayEntry) GetIconState() DesktopTrayIconState {
	if x == nil {
		return DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_UNSPECIFIED
	}
	return x.IconState
}

// GetSeverity returns the row severity.
func (x *DesktopTrayEntry) GetSeverity() DesktopTraySeverity {
	if x == nil {
		return DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_UNSPECIFIED
	}
	return x.Severity
}
