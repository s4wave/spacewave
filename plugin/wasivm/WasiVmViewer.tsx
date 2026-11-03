import { useCallback, useMemo, useState } from 'react'
import { LuMonitor, LuSquareTerminal } from '@s4wave/web/images/icons.js'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

import {
  useResource,
  useResourceValue,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import {
  TerminalPane,
  type TerminalPaneConnector,
} from '@s4wave/app/terminal/TerminalPane.js'
import type { SpaceContents } from '@s4wave/sdk/space/contents.js'
import { VmState } from '@s4wave/sdk/vm/v86.pb.js'
import {
  DEFAULT_WASIVM_INIT_PATH,
  PRED_WASIVM_IMAGE,
  PRED_WASIVM_ROOTFS,
  SET_WASIVM_STATE_OP_ID,
  WasiVmHandle,
  WasiVmTypeID,
} from '@s4wave/sdk/vm/wasivm.js'
import {
  SetWasiVmStateOp,
  WasiVm,
  WasiVmWindowList,
  type WasiVmWindow,
} from '@s4wave/sdk/vm/wasivm.pb.js'
import { iriToKey, keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { SpaceContentsContext } from '@s4wave/web/contexts/contexts.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'
import { Button } from '@s4wave/web/ui/button.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { useVmRequest, watchVmObject } from '@s4wave/app/vm/vm-object.js'
import { VmErrorBanner } from '@s4wave/app/vm/VmErrorBanner.js'
import { VmHeader, VmStopButton } from '@s4wave/app/vm/VmHeader.js'
import { VmViewSwitch } from '@s4wave/app/vm/VmViewSwitch.js'

import { WasiVmDisplay, type WasiVmDisplayConnector } from './WasiVmDisplay.js'
import { useWasiVmDownloads, WasiVmDownloadChip } from './WasiVmDownloadChip.js'
import { WasiVmMachineCard } from './WasiVmMachineCard.js'
import {
  useWasiVmWindowRoute,
  WasiVmWindowPane,
  WasiVmWindowPicker,
  WasiVmWindowsMenu,
  windowLabel,
  type WasiVmWindowView,
} from './WasiVmWindow.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  colors as themeColors,
} from '@s4wave/web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    flex: 1,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    minHeight: 0,
  },
  rule2: {
    display: { default: 'none', '@container (min-width: 28rem)': 'inline' },
  },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    justifyContent: 'center',
    height: '100%',
    width: '100%',
  },
  rule4: {
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule5: { padding: `calc(${themeLayout['--spacing']} * 3)`, flexShrink: 0 },
})

const stylexStyles = stylex.create({
  rule1: { maxWidth: 'var(--container-sm)', width: '100%' },
})

// readLinkedKey reads the key of the first object the VM at objectKey links to
// with predicate, or "" when it has none.
async function readLinkedKey(
  world: IWorldState,
  objectKey: string,
  predicate: string,
  signal: AbortSignal,
): Promise<string> {
  const resp = await world.lookupGraphQuads(
    keyToIRI(objectKey),
    predicate,
    undefined,
    undefined,
    1,
    signal,
  )
  const obj = resp.quads?.[0]?.obj
  return obj ? iriToKey(obj) : ''
}

// readRootfsKey reads the key of the object the VM at objectKey takes its
// root filesystem from: its own root filesystem, else its image, else "".
async function readRootfsKey(
  world: IWorldState,
  objectKey: string,
  signal: AbortSignal,
): Promise<string> {
  const rootfs = await readLinkedKey(
    world,
    objectKey,
    PRED_WASIVM_ROOTFS,
    signal,
  )
  return rootfs || readLinkedKey(world, objectKey, PRED_WASIVM_IMAGE, signal)
}

// requestWasiVmState asks the VM at objectKey for a target state. A start
// first approves the VM's process binding, which has the Space run the VM;
// the approval outlasts a stop. A start needs contents to approve the
// binding.
async function requestWasiVmState(
  world: IWorldState,
  contents: SpaceContents | null | undefined,
  objectKey: string,
  target: VmState,
): Promise<void> {
  if (target === VmState.VmState_RUNNING) {
    await contents?.setProcessBinding(objectKey, WasiVmTypeID, true)
  }
  const op = SetWasiVmStateOp.create({ objectKey, state: target })
  await world.applyWorldOp(
    SET_WASIVM_STATE_OP_ID,
    SetWasiVmStateOp.toBinary(op),
    '',
  )
}

// describeConsoleFailure states why the console of a VM could not attach.
function describeConsoleFailure(rawError?: string): string {
  return rawError
    ? `The console disconnected: ${rawError}`
    : 'The console ended before the VM wrote anything. Retry to attach again.'
}

// useWasiVm watches the VM at objectKey and reads its root filesystem, with
// defaults for every field not yet loaded.
function useWasiVm(
  worldState: ObjectViewerComponentProps['worldState'],
  objectKey: string,
) {
  // Watch the VM and read the root filesystem it runs on.
  const vmResource = useStreamingResource(
    worldState,
    useCallback(
      (world: IWorldState, signal: AbortSignal) =>
        watchVmObject(world, objectKey, (b) => WasiVm.fromBinary(b), signal),
      [objectKey],
    ),
    [],
  )
  const rootfsResource = useResource(
    worldState,
    (world: IWorldState, signal: AbortSignal) =>
      readRootfsKey(world, objectKey, signal),
    [objectKey],
  )

  // Expose the mounted VM's configuration and its current loading result.
  const vm = vmResource.value ?? WasiVm.create()
  const initPath = vm.config?.initPath || DEFAULT_WASIVM_INIT_PATH
  return {
    loaded: !!vmResource.value,
    loadError: vmResource.error,
    name: vm.name || objectKey,
    state: vm.observedState ?? VmState.VmState_STOPPED,
    desired: vm.state ?? VmState.VmState_STOPPED,
    errorMessage: vm.errorMessage?.trim() ?? '',
    runGeneration: vm.runGeneration ?? 0n,
    rootfsKey: rootfsResource.value ?? '',
    init: [initPath, ...(vm.config?.initArgs ?? [])].join(' '),
  }
}

// screenViews lists the panels of a running VM.
const screenViews = [
  { view: 'console', label: 'Console', icon: LuSquareTerminal },
  { view: 'display', label: 'Display', icon: LuMonitor },
] as const

// useWasiVmWindows watches the windows of the VM's Wayland display through
// its handle, starting again for each run. It is undefined until the run
// lists its windows.
function useWasiVmWindows(
  handleResource: Resource<WasiVmHandle>,
  runGeneration: bigint,
): WasiVmWindow[] | undefined {
  const list = useStreamingResource(
    handleResource,
    (handle: WasiVmHandle, signal: AbortSignal) => handle.watchWindows(signal),
    [runGeneration],
    { equals: WasiVmWindowList.equals },
  ).value
  return list ? (list.windows ?? []) : undefined
}

// WasiVmScreenProps configures the screen of one run of a VM.
interface WasiVmScreenProps {
  handle: WasiVmHandle | undefined
  showDisplay: boolean
}

// WasiVmScreen shows the console or the display of a running VM, each
// attached through the VM's typed resource.
function WasiVmScreen({ handle, showDisplay }: WasiVmScreenProps) {
  const connectConsole = useMemo<TerminalPaneConnector | undefined>(
    () =>
      handle
        ? (frames, signal) => handle.connectConsole(frames, signal)
        : undefined,
    [handle],
  )
  const connectDisplay = useMemo<WasiVmDisplayConnector | undefined>(
    () =>
      handle
        ? (input, signal) => handle.connectDisplay(input, signal)
        : undefined,
    [handle],
  )
  return showDisplay ? (
    <WasiVmDisplay connectDisplay={connectDisplay} />
  ) : (
    <TerminalPane
      session="console"
      connectTerminal={connectConsole}
      describeFailure={describeConsoleFailure}
    />
  )
}

// WasiVmTransitionCardProps describes a VM moving between states.
interface WasiVmTransitionCardProps {
  title: string
  detail: string
}

// WasiVmTransitionCard shows a VM that is starting or stopping.
function WasiVmTransitionCard({ title, detail }: WasiVmTransitionCardProps) {
  return (
    <div sx={stylexStyles2.rule1}>
      <LoadingCard
        view={{ state: 'active', title, detail }}
        sx={stylexStyles.rule1}
      />
    </div>
  )
}

// WasiVmShowDesktopButton returns a window's tab to the VM's view, which
// returns the window to the desktop. Its label collapses to the icon in a
// narrow header.
function WasiVmShowDesktopButton({ onClick }: { onClick: () => void }) {
  return (
    <Button
      variant="outline"
      size="xs"
      aria-label="Show desktop"
      onClick={onClick}
    >
      <LuMonitor />
      <span sx={stylexStyles2.rule2}>Desktop</span>
    </Button>
  )
}

// WasiVmLoadingView holds the view until the VM record loads, so no default
// stands in for it.
function WasiVmLoadingView({ loadError }: { loadError?: Error | null }) {
  return (
    <div sx={stylexStyles2.rule3}>
      {loadError ? (
        <VmErrorBanner
          title="Could not load this virtual machine"
          message={loadError.message}
        />
      ) : (
        <LoadingCard
          view={{
            state: 'active',
            title: 'Loading virtual machine',
            detail: 'Reading its settings from this Space.',
          }}
          sx={stylexStyles.rule1}
        />
      )}
    </div>
  )
}

// WasiVmRunningBodyProps configures the body of a running VM.
interface WasiVmRunningBodyProps {
  name: string
  handle: WasiVmHandle | undefined
  runGeneration: bigint
  windows: WasiVmWindow[] | undefined
  view: WasiVmWindowView
  showDisplay: boolean
}

// WasiVmRunningBody shows a running VM's console or display, or at a
// window's viewer path that window alone. A window tab whose window is gone,
// closed in this run or ended with an earlier one, offers the run's windows
// instead. Each run is a new kernel, so a new run attaches a new screen.
function WasiVmRunningBody({
  name,
  handle,
  runGeneration,
  windows,
  view,
  showDisplay,
}: WasiVmRunningBodyProps) {
  // Choose the direct VM display when no window route is selected.
  const route = view.route
  if (!route) {
    return (
      <WasiVmScreen
        key={runGeneration.toString()}
        handle={handle}
        showDisplay={showDisplay}
      />
    )
  }
  const earlierRun = route.run !== runGeneration
  if (earlierRun || (windows && !view.shownWindow)) {
    return (
      <WasiVmWindowPicker
        name={name}
        earlierRun={earlierRun}
        windows={windows}
        onShowWindow={view.showWindow}
        onShowDesktop={view.showDesktop}
      />
    )
  }
  return (
    <WasiVmWindowPane
      key={`${runGeneration}/${route.id}`}
      handle={handle}
      id={route.id}
      onShowDesktop={view.showDesktop}
    />
  )
}

// WasiVmHeaderControlsProps configures the VM header's controls.
interface WasiVmHeaderControlsProps {
  state: VmState
  stopping: boolean
  windowView: boolean
  windows: WasiVmWindow[]
  showDisplay: boolean
  onShowDisplay: (showDisplay: boolean) => void
  onOpenWindow: (id: number) => void
  onShowDesktop: () => void
  onStop: () => void
}

// WasiVmHeaderControls offers Show desktop in a window's view, and in the VM
// view the windows menu, the panel switch and Stop in the states they apply.
function WasiVmHeaderControls({
  state,
  stopping,
  windowView,
  windows,
  showDisplay,
  onShowDisplay,
  onOpenWindow,
  onShowDesktop,
  onStop,
}: WasiVmHeaderControlsProps) {
  if (windowView) {
    return <WasiVmShowDesktopButton onClick={onShowDesktop} />
  }
  const running = state === VmState.VmState_RUNNING
  const active = running || state === VmState.VmState_STARTING
  return (
    <>
      {running && showDisplay && (
        <WasiVmWindowsMenu windows={windows} onOpen={onOpenWindow} />
      )}
      {running && (
        <VmViewSwitch
          options={screenViews}
          view={showDisplay ? 'display' : 'console'}
          onViewChange={(view) => onShowDisplay(view === 'display')}
        />
      )}
      {active && <VmStopButton disabled={stopping} onStop={onStop} />}
    </>
  )
}

// WasiVmStoppedBodyProps configures the body of a VM that is not running.
interface WasiVmStoppedBodyProps {
  vm: ReturnType<typeof useWasiVm>
  operationError: string
  startDisabled: boolean
  onStart: () => void
  onReset: () => void
}

// WasiVmStoppedBody shows a starting or stopping VM's progress, or a stopped
// or failed VM's machine card.
function WasiVmStoppedBody({
  vm,
  operationError,
  startDisabled,
  onStart,
  onReset,
}: WasiVmStoppedBodyProps) {
  if (vm.state === VmState.VmState_STARTING) {
    return (
      <WasiVmTransitionCard
        title="Starting virtual machine"
        detail="Opening its root filesystem and booting the kernel."
      />
    )
  }
  if (vm.state === VmState.VmState_STOPPING) {
    return (
      <WasiVmTransitionCard
        title="Stopping virtual machine"
        detail="Waiting for the kernel to stop."
      />
    )
  }
  return (
    <WasiVmMachineCard
      {...vm}
      operationError={operationError}
      startDisabled={startDisabled}
      onStart={onStart}
      onReset={onReset}
    />
  )
}

// WasiVmViewer displays a WasiVM by the state its runtime observed. A stopped
// or failed VM shows its machine card with Start or Reset, a starting or
// stopping VM shows its progress, and a running VM shows its console or its
// display. At the viewer path of a window, a running VM shows that window
// alone, sized by the tab.
export default function WasiVmViewer({
  objectInfo,
  worldState,
  spaceContents,
}: ObjectViewerComponentProps) {
  // Resolve this VM's object, current resources and operation controls.
  const objectKey = getObjectKey(objectInfo)
  const contextContentsResource = SpaceContentsContext.useContext()
  const contents = useResourceValue(spaceContents ?? contextContentsResource)
  const world = worldState.value
  const vm = useWasiVm(worldState, objectKey)
  const [operationError, request] = useVmRequest()
  const [showDisplay, setShowDisplay] = useState(false)
  const handleResource = useAccessTypedHandle(
    worldState,
    objectKey,
    WasiVmHandle,
    WasiVmTypeID,
  )

  // Watch windows and downloads for this VM run before resolving its route.
  const windows = useWasiVmWindows(handleResource, vm.runGeneration)
  const downloads = useWasiVmDownloads(handleResource, vm.runGeneration)
  const view = useWasiVmWindowRoute(objectKey, vm.runGeneration, windows)
  useObjectName(view.shownWindow ? windowLabel(view.shownWindow) : vm.name)

  // setVmState requests a target state from the view.
  const setVmState = (target: VmState) => {
    if (!world) return
    request(() => requestWasiVmState(world, contents, objectKey, target))
  }

  // Wait for VM configuration before choosing its active or stopped view.
  if (!vm.loaded) {
    return <WasiVmLoadingView loadError={vm.loadError} />
  }
  const running = vm.state === VmState.VmState_RUNNING
  const active = running || vm.state === VmState.VmState_STARTING
  return (
    <div sx={stylexStyles2.rule4}>
      <VmHeader
        state={vm.state}
        activity={running && <WasiVmDownloadChip downloads={downloads} />}
      >
        <WasiVmHeaderControls
          state={vm.state}
          stopping={vm.desired === VmState.VmState_STOPPED}
          windowView={!!view.route}
          windows={windows ?? []}
          showDisplay={showDisplay}
          onShowDisplay={setShowDisplay}
          onOpenWindow={view.openWindow}
          onShowDesktop={view.showDesktop}
          onStop={() => setVmState(VmState.VmState_STOPPED)}
        />
      </VmHeader>
      {operationError && active && (
        <div sx={stylexStyles2.rule5}>
          <VmErrorBanner title="Operation error" message={operationError} />
        </div>
      )}
      {running ? (
        <WasiVmRunningBody
          name={vm.name}
          handle={handleResource.value ?? undefined}
          runGeneration={vm.runGeneration}
          windows={windows}
          view={view}
          showDisplay={showDisplay}
        />
      ) : (
        <WasiVmStoppedBody
          vm={vm}
          operationError={operationError}
          startDisabled={!world || !contents}
          onStart={() => setVmState(VmState.VmState_RUNNING)}
          onReset={() => setVmState(VmState.VmState_STOPPED)}
        />
      )}
    </div>
  )
}
