// Entry module for the spacewave-web plugin.
//
// This file triggers webPkg subpath discovery during the Vite build.
// Every @s4wave/web subpath used by any plugin must be imported here
// so the webPkg builder includes it as a named entrypoint.
//
// Without these imports, plugins with exclude: true would get 404s
// at runtime for subpaths not built into the web package output.

// command/
import '@s4wave/web/command/CommandPalette.js'
import '@s4wave/web/command/index.js'
import '@s4wave/web/command/KeyboardManager.js'
import '@s4wave/web/command/useCommand.js'

// contexts/
import '@s4wave/web/contexts/contexts.js'
import '@s4wave/web/contexts/SpaceContainerContext.js'
import '@s4wave/web/contexts/TabActiveContext.js'

// debug/
import '@s4wave/web/debug/CanvasGraphLinksDebug.js'
import '@s4wave/web/debug/DebugBridgeProvider.js'
import '@s4wave/web/debug/HDRDebug.js'
import '@s4wave/web/debug/LayoutColorsDebug.js'
import '@s4wave/web/debug/LayoutDebug.js'
import '@s4wave/web/debug/SessionSettingsDebug.js'

// devtools/
import '@s4wave/web/devtools/index.js'

// editors/
import '@s4wave/web/editors/file-browser/FileList.js'
import '@s4wave/web/editors/file-browser/FileListEntry.js'
import '@s4wave/web/editors/file-browser/Toolbar.js'
import '@s4wave/web/editors/file-browser/types.js'

// forge/
import '@s4wave/web/forge/StateBadge.js'
import '@s4wave/web/forge/useForgeBlockData.js'

// frame/
import '@s4wave/web/frame/bottom-bar-context.js'
import '@s4wave/web/frame/bottom-bar-item.js'
import '@s4wave/web/frame/bottom-bar-level.js'
import '@s4wave/web/frame/bottom-bar-root.js'
import '@s4wave/web/frame/bottom-icon-props.js'
import '@s4wave/web/frame/ViewerFrame.js'

// hooks/
import '@s4wave/web/hooks/useAccessTypedHandle.js'
import '@s4wave/web/hooks/usePromise.js'
import '@s4wave/web/hooks/useRootResource.js'
import '@s4wave/web/hooks/useUnixFSHandle.js'
import '@s4wave/web/hooks/useViewerRegistry.js'

// images/
import '@s4wave/web/images/AppLogo.js'

// layout/
import '@s4wave/web/layout/BaseLayout.js'
import '@s4wave/web/layout/layout.js'

// object/
import '@s4wave/web/object/ComponentSelector.js'
import '@s4wave/web/object/LayoutObjectViewer.js'
import '@s4wave/web/object/object.js'
import '@s4wave/web/object/object.pb.js'
import '@s4wave/web/object/ObjectViewer.js'
import '@s4wave/web/object/ObjectViewerContext.js'
import '@s4wave/web/object/TabContext.js'

// router/
import '@s4wave/web/router/app-path.js'
import '@s4wave/web/router/HashRouter.js'
import '@s4wave/web/router/HistoryRouter.js'
import '@s4wave/web/router/NavigatePath.js'
import '@s4wave/web/router/Redirect.js'
import '@s4wave/web/router/router.js'
import '@s4wave/web/router/static-routes.js'

// state/
import '@s4wave/web/state/global.js'
import '@s4wave/web/state/index.js'
import '@s4wave/web/state/interaction.js'
import '@s4wave/web/state/persist.js'
import '@s4wave/web/state/useStateAtomResource.js'

// style/
import '@s4wave/web/style/utils.js'

// test/
import '@s4wave/web/test/e2e-client.js'

// ui/
import '@s4wave/web/ui/badge.js'
import '@s4wave/web/ui/button.js'
import '@s4wave/web/ui/CollapsibleSection.js'
import '@s4wave/web/ui/command.js'
import '@s4wave/web/ui/dialog.js'
import '@s4wave/web/ui/DropdownMenu.js'
import '@s4wave/web/ui/EmptyState.js'
import '@s4wave/web/ui/ErrorState.js'
import '@s4wave/web/ui/FloatingWindow.js'
import '@s4wave/web/ui/list/ListState.js'
import '@s4wave/web/ui/login-form.js'
import '@s4wave/web/ui/ObjectKeySelector.js'
import '@s4wave/web/ui/Popover.js'
import '@s4wave/web/ui/RadioOption.js'
import '@s4wave/web/ui/shine-border.js'
import '@s4wave/web/ui/shooting-stars.js'
import '@s4wave/web/ui/toaster.js'
import '@s4wave/web/ui/tooltip.js'
import '@s4wave/web/ui/tree/TreeNode.js'
import '@s4wave/web/ui/tree/index.js'
import '@s4wave/web/ui/turnstile.js'
