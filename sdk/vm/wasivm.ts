import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import type { MessageStream } from 'starpc'

import type { TerminalFrame } from '../terminal/terminal.pb.js'
import type {
  WasiVmScene,
  WasiVmDisplayInput,
  WasiVmDownloads,
  WasiVmWindowInput,
  WasiVmWindowList,
} from './wasivm.pb.js'
import {
  WasiVmConsoleServiceClient,
  WasiVmDisplayServiceClient,
  WasiVmDownloadServiceClient,
} from './wasivm_srpc.pb.js'

// WasiVmTypeID is the object type identifier for WasiVm objects. It mirrors
// WasiVmTypeID in sdk/vm/wasivm.go.
export const WasiVmTypeID = 'vm/wasivm'

// CREATE_WASIVM_OP_ID is the operation type ID for CreateWasiVmOp.
export const CREATE_WASIVM_OP_ID = 'vm/wasivm/create'

// SET_WASIVM_STATE_OP_ID is the operation type ID for SetWasiVmStateOp.
export const SET_WASIVM_STATE_OP_ID = 'vm/wasivm/set-state'

// PRED_WASIVM_ROOTFS is the graph predicate linking a WasiVm to the UnixFS
// object holding its root filesystem. It mirrors PredWasiVmRootfs in
// sdk/vm/wasivm.go.
export const PRED_WASIVM_ROOTFS = 'wasivm/rootfs'

// PRED_WASIVM_IMAGE is the graph predicate linking a WasiVm to the
// WasiVmImage it boots when it has no root filesystem of its own. It mirrors
// PredWasiVmImage in sdk/vm/wasivm.go.
export const PRED_WASIVM_IMAGE = 'wasivm/image'

// WasiVmImageTypeID is the object type identifier for WasiVmImage objects.
// It mirrors WasiVmImageTypeID in sdk/vm/wasivm-image.go.
export const WasiVmImageTypeID = 'vm/image/wasivm'

// CREATE_WASIVM_IMAGE_OP_ID is the operation type ID for
// CreateWasiVmImageOp.
export const CREATE_WASIVM_IMAGE_OP_ID = 'vm/image/wasivm/create'

// PRED_WASIVM_IMAGE_ROOTFS is the graph predicate linking a WasiVmImage to
// the UnixFS object holding its root filesystem. It mirrors
// PredWasiVmImageRootfs in sdk/vm/wasivm-image.go.
export const PRED_WASIVM_IMAGE_ROOTFS = 'wasivmimage/rootfs'

// DEFAULT_WASIVM_INIT_PATH is the init program a WasiVm runs when its config
// selects no init program. It mirrors DefaultWasiVmInitPath in
// sdk/vm/wasivm.go.
export const DEFAULT_WASIVM_INIT_PATH = '/bin/sh'

// WasiVmHandle is the typed resource of a WasiVm object.
export class WasiVmHandle extends Resource {
  private console: WasiVmConsoleServiceClient
  private display: WasiVmDisplayServiceClient
  private downloads: WasiVmDownloadServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.console = new WasiVmConsoleServiceClient(resourceRef.client)
    this.display = new WasiVmDisplayServiceClient(resourceRef.client)
    this.downloads = new WasiVmDownloadServiceClient(resourceRef.client)
  }

  // connectConsole attaches to the console of the running VM. The stream
  // waits for the VM to start, replays its recent output, and ends with EXIT
  // when the VM stops.
  public connectConsole(
    frames: MessageStream<TerminalFrame>,
    abortSignal?: AbortSignal,
  ): MessageStream<TerminalFrame> {
    return this.console.Console(frames, abortSignal)
  }

  // connectDisplay attaches to the display of the running VM. The stream
  // waits for the VM and the first display it brings up, X display 0 or a
  // Wayland client of its compositor, sends the whole screen, then each region
  // that changes. Input carries the viewer's pointer and keys back.
  public connectDisplay(
    input: MessageStream<WasiVmDisplayInput>,
    abortSignal?: AbortSignal,
  ): MessageStream<WasiVmScene> {
    return this.display.Display(input, abortSignal)
  }

  // watchWindows streams the windows of the running VM's Wayland display,
  // then the list again each time a window maps, unmaps, is renamed, or moves
  // between the desktop and a viewer. It waits for the VM to start and ends
  // when the display does.
  public watchWindows(
    abortSignal?: AbortSignal,
  ): MessageStream<WasiVmWindowList> {
    return this.display.WatchWindows({}, abortSignal)
  }

  // connectWindow pops one window of the running VM's Wayland display out to
  // the viewer. The first input opens the window at the viewer's size.
  // Subsequent inputs resize the window or carry the viewer's pointer and
  // keys. The stream sends the window's frames as connectDisplay does and
  // ends when the window closes. The window returns to the desktop when the
  // viewer leaves.
  public connectWindow(
    input: MessageStream<WasiVmWindowInput>,
    abortSignal?: AbortSignal,
  ): MessageStream<WasiVmScene> {
    return this.display.Window(input, abortSignal)
  }

  // watchDownloads streams what the running VM has downloaded for its root
  // filesystem, then again each time a read from the CDN starts or finishes.
  // It waits for the VM to start and ends when the run does.
  public watchDownloads(
    abortSignal?: AbortSignal,
  ): MessageStream<WasiVmDownloads> {
    return this.downloads.WatchDownloads({}, abortSignal)
  }
}
