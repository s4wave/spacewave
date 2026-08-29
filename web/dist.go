package app_web

import "embed"

// DistSources contains the source closure that downstream Bldr apps use to
// resolve @s4wave/web during typechecking. The closure is wider than the
// spacewave-web manifest entrypoint list because public entrypoints import
// internal source directories and assets.
//
//go:embed command/CommandContext.tsx command/CommandPalette.tsx command/FocusContext.tsx
//go:embed command/KeyDispatcher.tsx command/KeybindingBindingsSection.tsx
//go:embed command/KeybindingCommandDetails.tsx command/KeybindingCommandList.tsx
//go:embed command/KeybindingConflictList.tsx command/KeybindingDiscoverySettings.tsx
//go:embed command/KeybindingEditor.tsx command/KeybindingEditorContext.ts
//go:embed command/KeybindingResolver.ts command/KeyboardManager.tsx
//go:embed command/WhichKeyPanel.tsx command/component.ts command/index.ts command/keybinding-editor-helpers.ts
//go:embed command/keybinding-overrides.ts command/sub-item-navigation.ts
//go:embed command/useAccountKeybindingOverrides.ts command/useCommand.ts command/useKeybindingEditorLayers.ts
//go:embed command/useKeybindingEditorModel.ts command/useKeybindingGraph.ts command/useKeybindingRecorder.ts
//go:embed command/useLocalKeybindingOverrides.ts command/useSpaceKeybindingOverrides.ts
//go:embed configtype/ConfigTypeRegistryContext.tsx configtype/configtype.ts configtype/useConfigEditor.tsx
//go:embed contexts/SessionContext.tsx contexts/SessionIndexContext.tsx contexts/SpaceContainerContext.tsx
//go:embed contexts/SpacewaveOnboardingContext.tsx contexts/SpacewaveOrgListContext.tsx
//go:embed contexts/TabActiveContext.tsx contexts/contexts.tsx contexts/index.ts debug/CanvasGraphLinksDebug.tsx
//go:embed debug/DebugBridgeProvider.tsx debug/DebugDbBench.tsx debug/ForgeViewerDebug.tsx debug/HDRDebug.tsx
//go:embed debug/LayoutColorsDebug.tsx debug/LayoutDebug.tsx debug/LoadingDebug.tsx
//go:embed debug/SessionSettingsDebug.tsx debug/UnixFSBrowserDebug.tsx devtools/CacheSeedTab.tsx
//go:embed devtools/ResourceDetailsPanel.tsx devtools/ResourceDevTools.tsx devtools/ResourceErrorsTab.tsx
//go:embed devtools/ResourceTreeTab.tsx devtools/StateDetailsPanel.tsx devtools/StateDevTools.tsx
//go:embed devtools/StateDevToolsContext.tsx devtools/StateTreeTab.tsx devtools/index.ts
//go:embed devtools/useStateInspectorEntries.ts dnd/app-drag.ts dnd/download-url-drag.ts download.ts
//go:embed editors/file-browser/FileList.tsx editors/file-browser/FileListEntry.tsx
//go:embed editors/file-browser/FileListState.tsx editors/file-browser/PathBar.tsx
//go:embed editors/file-browser/Toolbar.tsx editors/file-browser/types.ts forge/ForgeEntityLink.tsx
//go:embed forge/ForgeEntityList.tsx forge/ForgeViewerShell.tsx forge/StateBadge.tsx forge/predicates.ts
//go:embed forge/useForgeBlockData.ts forge/useForgeLinkedEntities.ts frame/ViewerFrame.tsx frame/bar.tsx
//go:embed frame/bottom-bar-context.tsx frame/bottom-bar-item.tsx frame/bottom-bar-level.tsx
//go:embed frame/bottom-bar-root.tsx frame/bottom-icon-props.tsx frame/breadcrumb-separator.tsx frame/frame.tsx
//go:embed hooks/useAccessTypedHandle.ts hooks/useContainerDensity.ts hooks/useDynamicRegistrations.ts
//go:embed hooks/useEmailManagement.ts hooks/useMobile.ts hooks/useMountAccount.ts
//go:embed hooks/useObjectTypeMetadata.ts hooks/usePromise.tsx hooks/useRootResource.tsx hooks/useSessionInfo.ts
//go:embed hooks/useTypedObjectState.ts hooks/useUnixFSHandle.tsx hooks/useViewerRegistry.tsx images/AppLogo.tsx
//go:embed images/spacewave-icon.png launcher/UpdateNotifier.tsx layout/BaseLayout.tsx
//go:embed layout/BaseLayoutContext.tsx layout/FlexTabContextMenu.tsx layout/LocalStorageLayout.tsx
//go:embed layout/layout.ts object/ComponentSelector.tsx object/DebugObjectViewer.tsx
//go:embed object/LayoutObjectViewer.tsx object/ObjectLink.tsx object/ObjectViewer.tsx
//go:embed object/ObjectViewerContent.tsx object/ObjectViewerContext.tsx object/ObjectViewerDetails.tsx
//go:embed object/ObjectViewerLoadingState.tsx object/ObjectViewerNotFoundState.tsx object/TabContent.tsx
//go:embed object/TabContext.tsx object/ViewerStatusShell.tsx object/layout-object-app-drag.ts
//go:embed object/object-viewer-space-actions.ts object/object.pb.ts object/object.ts object/useObjectViewer.tsx
//go:embed object/useObjectViewerSetup.ts platform/detect-platform.ts router/HashRouter.tsx
//go:embed router/HistoryRouter.tsx router/NavigatePath.tsx router/Redirect.tsx router/app-path.ts
//go:embed router/hash.tsx router/router.tsx router/static-routes.ts sdk/app/SpacewaveRuntimeProviders.tsx
//go:embed sdk/app/base-viewers.ts sdk/app/index.ts sdk/app/lifecycle.tsx sdk/app/viewer-catalog.ts
//go:embed space/object-tree.tsx space/space-object-navigation-actions.ts state/StateAtomRegistry.tsx
//go:embed state/global.ts state/index.tsx state/interaction.ts state/persist.tsx state/useBackendStateAtom.tsx
//go:embed state/useStateAtomResource.tsx style/app.css style/flexlayout/base.css
//go:embed style/flexlayout/flexlayout.css style/shadcn.css style/utils.ts title/DocumentTitleContext.tsx
//go:embed title/DocumentTitleFocusContext.ts transform/TransformConfigDisplay.tsx ui/BackButton.tsx
//go:embed ui/CollapsibleSection.tsx ui/CopyButton.tsx ui/CopyableField.tsx ui/DashboardButton.tsx
//go:embed ui/DropdownMenu.tsx ui/DropdownMenuGhostAnchor.tsx ui/DropdownTriggerButton.tsx ui/EmptyState.tsx
//go:embed ui/ErrorState.tsx ui/ExperimentalBadge.tsx ui/FloatingWindow.tsx ui/InfoCard.tsx ui/MenuButton.tsx
//go:embed ui/MenuButtonGroup.tsx ui/Menubar.tsx ui/ObjectKeySelector.tsx ui/OutputPanel.tsx ui/PanelHeader.tsx
//go:embed ui/PanelSizeGate.tsx ui/Popover.tsx ui/RadioOption.tsx ui/SearchBox.tsx ui/StatCard.tsx
//go:embed ui/StatsBar.tsx ui/StatusList.tsx ui/Window.tsx ui/badge.tsx ui/button-group.tsx ui/button.tsx
//go:embed ui/card.tsx ui/command.tsx ui/credential/AuthProgressCard.tsx ui/credential/CredentialProofInput.tsx
//go:embed ui/credential/auth-utils.ts ui/credential/useCredentialProof.ts ui/dialog.tsx ui/input.tsx
//go:embed ui/label.tsx ui/list/List.tsx ui/list/ListItem.tsx ui/list/ListRow.tsx ui/list/ListState.tsx
//go:embed ui/list/index.ts ui/loading/LoadingCard.tsx ui/loading/LoadingInline.tsx ui/loading/LoadingScreen.tsx
//go:embed ui/loading/ProgressBar.tsx ui/loading/Spinner.tsx ui/loading/index.ts ui/loading/types.ts
//go:embed ui/loading/useReducedMotion.ts ui/login-form.tsx ui/path/PathInput.tsx ui/path/index.ts
//go:embed ui/range-slider.tsx ui/separator.tsx ui/sheet.tsx ui/shine-border.tsx ui/tabs.tsx ui/toaster.tsx
//go:embed ui/tooltip.tsx ui/tree/Tree.tsx ui/tree/TreeNode.tsx ui/tree/TreeRow.tsx ui/tree/TreeState.tsx
//go:embed ui/tree/index.ts ui/turnstile.tsx util/isEqual.ts
//go:embed react-css.d.ts
var DistSources embed.FS
