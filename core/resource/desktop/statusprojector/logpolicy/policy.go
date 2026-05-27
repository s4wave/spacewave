package logpolicy

import desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"

// Level names the severity used for a desktop tray projection log decision.
type Level uint8

const (
	// LevelDebug logs routine projection churn at debug level.
	LevelDebug Level = iota
	// LevelInfo logs meaningful user-visible projection transitions at info level.
	LevelInfo
)

// Decision describes how to log a desktop tray projection publish result.
type Decision struct {
	Level   Level
	Message string
}

// Classify decides whether a desktop tray projection publish should be logged
// as routine churn or as a meaningful state transition.
func Classify(
	prev *desktop_runtime.DesktopRuntimeState,
	current *desktop_runtime.DesktopRuntimeState,
	changed bool,
) Decision {
	if current == nil || !changed {
		return Decision{
			Level:   LevelDebug,
			Message: "desktop tray projection unchanged",
		}
	}
	if prev == nil ||
		desktopRuntimeStatusClassChanged(prev, current) ||
		desktopRuntimeAttentionChanged(prev, current) ||
		desktopRuntimeUpdateChanged(prev, current) {
		return Decision{
			Level:   LevelInfo,
			Message: "published desktop tray projection",
		}
	}
	return Decision{
		Level:   LevelDebug,
		Message: "desktop tray projection changed",
	}
}

func desktopRuntimeStatusClassChanged(
	prev *desktop_runtime.DesktopRuntimeState,
	current *desktop_runtime.DesktopRuntimeState,
) bool {
	return prev.GetLifecycle() != current.GetLifecycle() ||
		prev.GetHealth() != current.GetHealth() ||
		prev.GetStatusText() != current.GetStatusText()
}

func desktopRuntimeAttentionChanged(
	prev *desktop_runtime.DesktopRuntimeState,
	current *desktop_runtime.DesktopRuntimeState,
) bool {
	prevItems := prev.GetAttentionItems()
	currentItems := current.GetAttentionItems()
	if len(prevItems) != len(currentItems) {
		return true
	}
	for idx, prevItem := range prevItems {
		if !prevItem.EqualVT(currentItems[idx]) {
			return true
		}
	}
	return false
}

func desktopRuntimeUpdateChanged(
	prev *desktop_runtime.DesktopRuntimeState,
	current *desktop_runtime.DesktopRuntimeState,
) bool {
	prevUpdate := prev.GetUpdate()
	currentUpdate := current.GetUpdate()
	if prevUpdate == nil || currentUpdate == nil {
		return prevUpdate.GetReady() != currentUpdate.GetReady()
	}
	if prevUpdate.GetReady() != currentUpdate.GetReady() {
		return true
	}
	return currentUpdate.GetReady() && !prevUpdate.EqualVT(currentUpdate)
}
