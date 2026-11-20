// Package s4wave_layout is the contract between a tabbed layout and the host
// that renders it.
//
// LayoutHost streams the layout model (WatchLayoutModel) and accepts tab
// changes: AddTab, NavigateTab and ReplaceTab. The world subpackage stores an
// ObjectLayout as a World object, so a layout of panels and tabs persists with
// the Space and opens the same on every device.
package s4wave_layout
