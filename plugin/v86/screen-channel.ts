import type { ScreenAdapter, ScreenLayer } from '@aptre/v86/screen'
import type { KeyEvent } from '@aptre/v86'

// ScreenCall is one VGA call on the v86 screen adapter, as method name and
// arguments.
export type ScreenCall =
  | ['set_mode', boolean]
  | ['set_size_text', number, number]
  | ['set_size_graphical', number, number, number, number]
  | [
      'set_font_bitmap',
      number,
      boolean,
      boolean,
      boolean,
      Uint8Array | null,
      boolean,
    ]
  | ['set_font_page', number, number]
  | ['put_char', number, number, number, number, number, number]
  | ['update_cursor', number, number]
  | ['update_cursor_scanline', number, number, boolean]
  | ['clear_screen']
  | ['update_buffer', ScreenRows[]]

// ScreenRows is a framebuffer layer cut down to its changed rows. data holds
// rows buffer_y to buffer_y + buffer_height of a buffer width pixels wide.
export type ScreenRows = Omit<ScreenLayer, 'image_data' | 'buffer_y'> & {
  data: Uint8ClampedArray<ArrayBuffer>
  width: number
}

// ScreenFrame is the wire shape carried on the v86 screen channel. The
// runner sends "out" frames with the VGA calls of one emulator slice, and
// answers every fill request with one, empty when nothing changed. Viewers
// send "in" frames.
export type ScreenFrame =
  | { dir: 'out'; calls: ScreenCall[] }
  | { dir: 'in'; op: ScreenInput }

// ScreenInput is a request from a viewer to the runner: hello replays the
// whole screen, fill asks for the changed framebuffer rows, key presses or
// releases one key, release lets go of every pressed key, and ctrl-alt-del
// sends that chord.
export type ScreenInput =
  | { kind: 'hello' }
  | { kind: 'fill' }
  | { kind: 'key'; key: KeyEvent; down: boolean }
  | { kind: 'release' }
  | { kind: 'ctrl-alt-del' }

// ScreenEmulator is the part of the v86 emulator the screen relay drives.
export type ScreenEmulator = {
  screen_fill_buffer(): void
  screen_redraw(): void
  keyboard_send_event(key: KeyEvent, down: boolean): void
  keyboard_release_all(): void
  keyboard_send_scancodes(codes: number[]): Promise<void>
}

// replayOrder lists the screen state calls in the order a viewer applies
// them: mode before sizes, font before the cursor.
const replayOrder = [
  'set_mode',
  'set_size_text',
  'set_size_graphical',
  'set_font_bitmap',
  'set_font_page',
  'update_cursor_scanline',
  'update_cursor',
] as const

type ReplayCall = (typeof replayOrder)[number]

// ctrlAltDel presses and releases Ctrl, Alt and Delete as PS/2 scancodes.
const ctrlAltDel = [0x1d, 0x38, 0xe0, 0x53, 0xe0, 0xd3, 0xb8, 0x9d]

// v86ScreenChannelName returns the BroadcastChannel name that relays the
// screen and keyboard of the VmV86 instance at vmObjectKey.
export function v86ScreenChannelName(vmObjectKey: string): string {
  return `v86-screen-${vmObjectKey}`
}

// ScreenRelay is the screen adapter of a v86 emulator that runs without a
// DOM. It forwards each VGA call to the viewers on the screen channel, one
// frame per emulator slice, and keeps the screen state so a viewer that
// opens later receives the whole screen. It also applies viewer key input.
export class ScreenRelay {
  FLAG_BLINKING = 0x01
  FLAG_FONT_PAGE_B = 0x02

  private calls: ScreenCall[] = []
  private flushQueued = false
  private state = new Map<ReplayCall, ScreenCall>()
  // fontPlane is the relay's copy of the last font plane, replayed with the
  // set_font_bitmap state.
  private fontPlane: Uint8Array | null = null
  private emulator: ScreenEmulator | undefined

  constructor(private channel: BroadcastChannel) {
    channel.onmessage = (ev: MessageEvent<ScreenFrame>) => {
      if (ev.data?.dir === 'in') this.input(ev.data.op)
    }
  }

  // attach connects the relay to the emulator it draws for. Input that
  // arrives before attach is dropped.
  attach(emulator: ScreenEmulator): void {
    this.emulator = emulator
  }

  set_mode(graphical: boolean): void {
    this.record(['set_mode', graphical])
  }

  set_size_text(cols: number, rows: number): void {
    this.record(['set_size_text', cols, rows])
  }

  set_size_graphical(
    width: number,
    height: number,
    bufferWidth: number,
    bufferHeight: number,
  ): void {
    this.record([
      'set_size_graphical',
      width,
      height,
      bufferWidth,
      bufferHeight,
    ])
  }

  // set_font_bitmap sends the 64 KiB font plane only when it changed; the
  // viewer keeps the last plane. The state keeps its own copy for replay.
  set_font_bitmap(
    height: number,
    width9px: boolean,
    widthDbl: boolean,
    copy8thCol: boolean,
    bitmap: Uint8Array,
    bitmapChanged: boolean,
  ): void {
    // Keep the font with its own copy of the plane for replay.
    const prev = this.fontPlane
    const plane = bitmapChanged || !prev ? bitmap.slice() : prev
    const fresh = plane !== prev
    this.fontPlane = plane
    this.state.set('set_font_bitmap', [
      'set_font_bitmap',
      height,
      width9px,
      widthDbl,
      copy8thCol,
      plane,
      true,
    ])

    // Send the plane only when the viewers lack it.
    this.push([
      'set_font_bitmap',
      height,
      width9px,
      widthDbl,
      copy8thCol,
      fresh ? plane : null,
      fresh,
    ])
  }

  set_font_page(pageA: number, pageB: number): void {
    this.record(['set_font_page', pageA, pageB])
  }

  put_char(
    row: number,
    col: number,
    chr: number,
    flags: number,
    bg: number,
    fg: number,
  ): void {
    this.push(['put_char', row, col, chr, flags, bg, fg])
  }

  update_cursor(row: number, col: number): void {
    this.record(['update_cursor', row, col])
  }

  update_cursor_scanline(start: number, end: number, enabled: boolean): void {
    this.record(['update_cursor_scanline', start, end, enabled])
  }

  clear_screen(): void {
    this.push(['clear_screen'])
  }

  // update_buffer copies the changed rows out of each layer, which views the
  // emulator's memory.
  update_buffer(layers: ScreenLayer[]): void {
    this.push(['update_buffer', layers.map(copyRows)])
  }

  pause(): void {}

  continue(): void {}

  set_scale(): void {}

  destroy(): void {}

  get_text_screen(): string[] {
    return []
  }

  // input applies one viewer request. A fill is answered even before the
  // emulator attaches, so the viewer never waits on it.
  private input(op: ScreenInput): void {
    const emulator = this.emulator
    switch (op.kind) {
      case 'hello':
        for (const name of replayOrder) {
          const call = this.state.get(name)
          if (call) this.push(call)
        }
        emulator?.screen_redraw()
        return
      case 'fill':
        emulator?.screen_fill_buffer()
        this.queueFlush()
        return
      case 'key':
        emulator?.keyboard_send_event(op.key, op.down)
        return
      case 'release':
        emulator?.keyboard_release_all()
        return
      case 'ctrl-alt-del':
        void emulator?.keyboard_send_scancodes(ctrlAltDel)
        return
    }
  }

  // record keeps a state call for replay and sends it.
  private record(call: Extract<ScreenCall, [ReplayCall, ...unknown[]]>): void {
    this.state.set(call[0], call)
    this.push(call)
  }

  private push(call: ScreenCall): void {
    this.calls.push(call)
    this.queueFlush()
  }

  private queueFlush(): void {
    if (this.flushQueued) return
    this.flushQueued = true
    queueMicrotask(() => this.flush())
  }

  private flush(): void {
    const frame: ScreenFrame = { dir: 'out', calls: this.calls }
    this.flushQueued = false
    this.calls = []
    this.channel.postMessage(frame)
  }
}

// copyRows copies the changed rows of one framebuffer layer.
function copyRows(layer: ScreenLayer): ScreenRows {
  const width = layer.image_data.width
  const start = layer.buffer_y * width * 4
  return {
    data: layer.image_data.data.slice(
      start,
      start + layer.buffer_height * width * 4,
    ),
    width,
    screen_x: layer.screen_x,
    screen_y: layer.screen_y,
    buffer_x: layer.buffer_x,
    buffer_width: layer.buffer_width,
    buffer_height: layer.buffer_height,
  }
}

// ScreenView draws the relayed screen of one VM into a v86 ScreenAdapter.
export type ScreenView = {
  // key presses or releases one key in the guest.
  key(key: KeyEvent, down: boolean): void
  // release lets go of every key the viewer pressed.
  release(): void
  // close stops drawing and closes the channel.
  close(): void
}

// attachScreenView draws the screen of the VM at vmObjectKey into a new v86
// ScreenAdapter inside container. The adapter's frame timer asks the runner
// for changed framebuffer rows, at most one request in flight. Call close to
// release the adapter and the channel.
export function attachScreenView(
  Adapter: typeof ScreenAdapter,
  container: HTMLElement,
  vmObjectKey: string,
): ScreenView {
  // Open the channel and the adapter. The adapter's timer pulls frames; any
  // frame from the runner ends the wait, so a request lost to a stopped
  // runner does not stall a restarted one.
  const channel = new BroadcastChannel(v86ScreenChannelName(vmObjectKey))
  const send = (op: ScreenInput) =>
    channel.postMessage({ dir: 'in', op } satisfies ScreenFrame)
  let fillPending = false
  const adapter = new Adapter({ container, use_graphical_text: true }, () => {
    if (fillPending) return
    fillPending = true
    send({ kind: 'fill' })
  })

  // Apply the runner's calls, restoring the font plane the runner omits.
  let fontPlane: Uint8Array | null = null
  channel.onmessage = (ev: MessageEvent<ScreenFrame>) => {
    const frame = ev.data
    if (frame?.dir !== 'out') return
    fillPending = false
    for (const call of frame.calls) {
      if (call[0] === 'set_font_bitmap') {
        fontPlane = call[5] ?? fontPlane
        if (fontPlane) {
          adapter.set_font_bitmap(
            call[1],
            call[2],
            call[3],
            call[4],
            fontPlane,
            call[6],
          )
        }
      } else if (call[0] === 'update_buffer') {
        adapter.update_buffer(call[1].map(toLayer))
      } else {
        const [name, ...args] = call
        ;(adapter[name] as (...args: unknown[]) => void)(...args)
      }
    }
  }

  // Ask the runner for the whole screen.
  send({ kind: 'hello' })

  return {
    key: (key, down) => send({ kind: 'key', key, down }),
    release: () => send({ kind: 'release' }),
    close() {
      adapter.destroy()
      channel.close()
    },
  }
}

// toLayer rebuilds a framebuffer layer from its relayed rows.
function toLayer(rows: ScreenRows): ScreenLayer {
  return {
    image_data: new ImageData(rows.data, rows.width, rows.buffer_height),
    screen_x: rows.screen_x,
    screen_y: rows.screen_y,
    buffer_x: rows.buffer_x,
    buffer_y: 0,
    buffer_width: rows.buffer_width,
    buffer_height: rows.buffer_height,
  }
}

// postScreenInput sends one request to the runner of the VM at vmObjectKey.
export function postScreenInput(vmObjectKey: string, op: ScreenInput): void {
  const channel = new BroadcastChannel(v86ScreenChannelName(vmObjectKey))
  channel.postMessage({ dir: 'in', op } satisfies ScreenFrame)
  channel.close()
}
