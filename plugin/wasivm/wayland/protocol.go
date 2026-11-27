package wayland

// The opcodes of the requests the compositor handles and the events it sends,
// from wayland.xml and xdg-shell.xml. Each name joins the interface and the
// message.

// wl_display.
const (
	displaySync        = 0
	displayGetRegistry = 1

	displayError    = 0
	displayDeleteID = 1
)

// wl_display error codes.
const (
	errInvalidObject  = 0
	errInvalidMethod  = 1
	errImplementation = 3
)

// wl_registry.
const (
	registryBind = 0

	registryGlobal = 0
)

// callbackDone is the wl_callback done event opcode.
const callbackDone = 0

// wl_compositor.
const (
	compositorCreateSurface = 0
	compositorCreateRegion  = 1
)

// wl_surface.
const (
	surfaceDestroy         = 0
	surfaceAttach          = 1
	surfaceDamage          = 2
	surfaceFrame           = 3
	surfaceSetOpaqueRegion = 4
	surfaceSetInputRegion  = 5
	surfaceCommit          = 6
	surfaceSetBufferScale  = 8
	surfaceDamageBuffer    = 9
)

// wl_surface events and errors.
const (
	surfaceEnter           = 0
	surfaceErrInvalidScale = 0
)

// wl_region.
const (
	regionDestroy  = 0
	regionAdd      = 1
	regionSubtract = 2
)

// wl_subcompositor.
const (
	subcompositorDestroy       = 0
	subcompositorGetSubsurface = 1
)

// wl_subsurface.
const (
	subsurfaceDestroy     = 0
	subsurfaceSetPosition = 1
	subsurfacePlaceAbove  = 2
	subsurfacePlaceBelow  = 3
)

// subcompositorErrBadSurface is a wl_subcompositor error code.
const subcompositorErrBadSurface = 0

// wl_shm.
const (
	shmCreatePool = 0

	shmFormat = 0
)

// wl_shm formats: 32-bit pixels with alpha or padding in the high byte.
const (
	shmFormatARGB8888 = 0
	shmFormatXRGB8888 = 1
)

// wl_shm error codes.
const (
	shmErrInvalidFormat = 0
	shmErrInvalidStride = 1
	shmErrInvalidFD     = 2
)

// wl_shm_pool.
const (
	shmPoolCreateBuffer = 0
	shmPoolDestroy      = 1
	shmPoolResize       = 2
)

// wl_buffer.
const (
	bufferDestroy = 0

	bufferRelease = 0
)

// wl_seat.
const (
	seatGetPointer  = 0
	seatGetKeyboard = 1
	seatGetTouch    = 2
	seatRelease     = 3

	seatCapabilities = 0
	seatName         = 1
)

// wl_seat capabilities.
const (
	seatCapPointer  = 1
	seatCapKeyboard = 2
)

// wl_pointer.
const (
	pointerRelease = 1

	pointerEnter  = 0
	pointerLeave  = 1
	pointerMotion = 2
	pointerButton = 3
	pointerAxis   = 4
	pointerFrame  = 5
)

// wl_keyboard.
const (
	keyboardRelease = 0

	keyboardKeymap     = 0
	keyboardEnter      = 1
	keyboardLeave      = 2
	keyboardKey        = 3
	keyboardModifiers  = 4
	keyboardRepeatInfo = 5
)

// keymapXKBV1 is the wl_keyboard keymap format: an xkbcommon text keymap.
const keymapXKBV1 = 1

// wl_output.
const (
	outputGeometry    = 0
	outputMode        = 1
	outputDone        = 2
	outputScale       = 3
	outputName        = 4
	outputDescription = 5

	outputRelease = 0
)

// outputModeCurrentPreferred is the wl_output mode flags for a current and
// preferred mode.
const outputModeCurrentPreferred = 3

// zxdg_output_manager_v1.
const (
	xdgOutputManagerDestroy      = 0
	xdgOutputManagerGetXdgOutput = 1
)

// zxdg_output_v1.
const (
	xdgOutputDestroy = 0

	xdgOutputLogicalPosition = 0
	xdgOutputLogicalSize     = 1
	xdgOutputDone            = 2
	xdgOutputName            = 3
	xdgOutputDescription     = 4
)

// wl_data_device_manager.
const (
	dataDeviceManagerCreateDataSource = 0
	dataDeviceManagerGetDataDevice    = 1
)

// xdg_wm_base.
const (
	wmBaseDestroy          = 0
	wmBaseCreatePositioner = 1
	wmBaseGetXdgSurface    = 2
)

// xdg_wm_base error codes.
const (
	wmBaseErrRole            = 0
	wmBaseErrInvalidPopup    = 3
	wmBaseErrInvalidSurface  = 4
	wmBaseErrInvalidPosition = 6
)

// xdg_positioner.
const (
	positionerDestroy       = 0
	positionerSetSize       = 1
	positionerSetAnchorRect = 2
	positionerSetAnchor     = 3
	positionerSetGravity    = 4
	positionerSetOffset     = 6
)

// xdg_positioner anchors and gravities, which share their values: the edge
// or corner of the anchor rectangle the popup attaches to, and the direction
// it extends from there.
const (
	edgeTop         = 1
	edgeBottom      = 2
	edgeLeft        = 3
	edgeRight       = 4
	edgeTopLeft     = 5
	edgeBottomLeft  = 6
	edgeTopRight    = 7
	edgeBottomRight = 8
)

// xdg_surface.
const (
	xdgSurfaceDestroy           = 0
	xdgSurfaceGetToplevel       = 1
	xdgSurfaceGetPopup          = 2
	xdgSurfaceSetWindowGeometry = 3

	xdgSurfaceConfigure = 0
)

// xdg_toplevel.
const (
	toplevelDestroy         = 0
	toplevelSetTitle        = 2
	toplevelSetAppID        = 3
	toplevelMove            = 5
	toplevelResize          = 6
	toplevelSetMaxSize      = 7
	toplevelSetMinSize      = 8
	toplevelSetMaximized    = 9
	toplevelUnsetMaximized  = 10
	toplevelSetFullscreen   = 11
	toplevelUnsetFullscreen = 12

	toplevelConfigure = 0
	toplevelClose     = 1
)

// xdg_toplevel states.
const (
	toplevelMaximized  = 1
	toplevelFullscreen = 2
	toplevelResizing   = 3
	toplevelActivated  = 4
)

// xdg_toplevel resize edges, as a mask.
const (
	resizeTop    = 1
	resizeBottom = 2
	resizeLeft   = 4
	resizeRight  = 8
)

// zxdg_decoration_manager_v1.
const (
	decorationManagerDestroy               = 0
	decorationManagerGetToplevelDecoration = 1
)

// zxdg_toplevel_decoration_v1.
const (
	decorationDestroy   = 0
	decorationSetMode   = 1
	decorationUnsetMode = 2

	decorationConfigure = 0
)

// zxdg_toplevel_decoration_v1 error codes.
const (
	decorationErrAlreadyConstructed = 1
)

// zxdg_toplevel_decoration_v1 modes: the client draws its decorations, or
// the compositor does.
const (
	decorationClientSide = 1
	decorationServerSide = 2
)

// xdg_popup.
const (
	popupDestroy = 0
	popupGrab    = 1

	popupConfigure = 0
	popupDone      = 1
)

// zwlr_layer_shell_v1.
const (
	layerShellGetLayerSurface = 0
	layerShellDestroy         = 1
)

// zwlr_layer_shell_v1 error codes.
const (
	layerShellErrRole         = 0
	layerShellErrInvalidLayer = 1
)

// zwlr_layer_shell_v1 layers, bottom to top, and their count.
const (
	layerBackground = 0
	layerBottom     = 1
	layerTop        = 2
	layerOverlay    = 3
	layerCount      = 4
)

// zwlr_layer_surface_v1.
const (
	layerSurfaceSetSize                  = 0
	layerSurfaceSetAnchor                = 1
	layerSurfaceSetExclusiveZone         = 2
	layerSurfaceSetMargin                = 3
	layerSurfaceSetKeyboardInteractivity = 4
	layerSurfaceGetPopup                 = 5
	layerSurfaceDestroy                  = 7
	layerSurfaceSetLayer                 = 8

	layerSurfaceConfigure = 0
)

// zwlr_layer_surface_v1 error codes.
const (
	layerSurfaceErrInvalidSize                  = 1
	layerSurfaceErrInvalidAnchor                = 2
	layerSurfaceErrInvalidKeyboardInteractivity = 3
)

// zwlr_layer_surface_v1 anchors, as a mask.
const (
	anchorTop    = 1
	anchorBottom = 2
	anchorLeft   = 4
	anchorRight  = 8
)

// zwlr_layer_surface_v1 keyboard interactivity: never focused, focused while
// mapped, or focused by a press.
const (
	interactivityNone      = 0
	interactivityExclusive = 1
	interactivityOnDemand  = 2
)
