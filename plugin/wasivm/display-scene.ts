import {
  WasiVmSceneFrameButtonKind,
  type WasiVmScene,
  type WasiVmSceneFrame,
  type WasiVmSceneLayer,
  type WasiVmSceneMove,
} from '@s4wave/sdk/vm/wasivm.pb.js'

// SceneScreen is the screen a scene covers: its size in units, the screen
// pixels per unit, and the background as RGBA in the high to low bytes.
export interface SceneScreen {
  width: number
  height: number
  scale: number
  background: number
}

// SceneRect is a rectangle of texture pixels.
export interface SceneRect {
  x: number
  y: number
  width: number
  height: number
}

// SceneTexture is the viewer's copy of one layer's texture: premultiplied
// RGBA rows, and the areas changed since a renderer last took them.
export interface SceneTexture {
  width: number
  height: number
  pixels: Uint8Array
  // dirty lists the areas the patches changed since takeDirty, or null when
  // the whole texture changed.
  dirty: SceneRect[] | null
}

// SceneLayer is a layer as the renderer draws it: its texture and where it
// covers the screen, in units.
export interface SceneLayer {
  id: number
  x: number
  y: number
  width: number
  height: number
  opaque: boolean
  texture: SceneTexture
}

// ScenePoint is a position on the screen in units.
export interface ScenePoint {
  x: number
  y: number
}

// FramePainter paints the textures of the desktop's window frames.
export interface FramePainter {
  // title paints the title bar of the frame f at scale screen pixels per
  // unit, as a texture of f.width by f.titleHeight units.
  title(f: WasiVmSceneFrame, scale: number): SceneTexture
  // edge paints the one pixel stretched around a frame to outline it.
  edge(): SceneTexture
}

// FramePart is the part of a window frame under the pointer: the title bar,
// a button, or the resize band at some edges.
export type FramePart =
  | { kind: 'title'; frame: WasiVmSceneFrame }
  | {
      kind: 'button'
      frame: WasiVmSceneFrame
      button: WasiVmSceneFrameButtonKind
    }
  | { kind: 'band'; frame: WasiVmSceneFrame; edges: number }

// edgeTop, edgeBottom, edgeLeft and edgeRight are the resize edges of a
// band, as the compositor numbers them.
export const edgeTop = 1
export const edgeBottom = 2
export const edgeLeft = 4
export const edgeRight = 8

// FrameTexture is a frame's painted title bar and the key of what it shows.
interface FrameTexture {
  key: string
  texture: SceneTexture
}

// Scene is the state a display stream builds: the screen, the layers bottom to
// top, the roots' positions, each layer's texture, the desktop's window frames
// and the window move in progress. apply folds each update into it, point
// takes the viewer's own pointer, and press starts a move on a title bar
// before the compositor answers. The renderer draws the scene.
export class Scene {
  // screen is the screen's size in units, its scale and its background.
  screen: SceneScreen = {
    width: 0,
    height: 0,
    scale: 1,
    background: 0x000000ff,
  }
  // layers is the layers, bottom to top, as the last update listing them
  // gave them.
  private layers: WasiVmSceneLayer[] = []
  // roots maps each root's id to its position in units.
  private roots = new Map<number, ScenePoint>()
  // move is the window move in progress the compositor reported, or null.
  private move: WasiVmSceneMove | null = null
  // predicted is the move the viewer started from its own press on a title
  // bar, until the compositor reports a grab or, once released, the roots.
  private predicted: { move: WasiVmSceneMove; released: boolean } | null = null
  // frames maps each framed root's id to its frame.
  private frames = new Map<number, WasiVmSceneFrame>()
  // frameTextures maps each framed root's id to its painted title bar.
  private frameTextures = new Map<number, FrameTexture>()
  // edge is the painted frame edge, or null before a frame is drawn.
  private edge: SceneTexture | null = null
  // pointer is the viewer's pointer in units, or null before it first moves.
  private pointer: ScenePoint | null = null
  // textures maps each layer's id to its texture.
  private textures = new Map<number, SceneTexture>()

  // constructor takes the painter of the frames' title bars. Without one the
  // scene draws no frames.
  constructor(private readonly paint?: FramePainter) {}

  // apply folds the update u into the scene: the screen, the layers, the
  // roots, the grab and the frames it sets, then its patches. A layer missing
  // from a new list loses its texture, and a layer whose texture size changed
  // gets an empty one. A grab, or roots after the release, end a predicted
  // move.
  apply(u: WasiVmScene) {
    // Take the screen and the roots.
    if (u.screen) {
      this.screen = {
        width: u.screen.width ?? 0,
        height: u.screen.height ?? 0,
        scale: u.screen.scale || 1,
        background: u.screen.background ?? 0,
      }
    }
    if (u.roots) {
      this.roots = new Map(
        (u.roots.roots ?? []).map((r) => [
          r.id ?? 0,
          { x: r.x ?? 0, y: r.y ?? 0 },
        ]),
      )
    }
    if (u.grab) {
      this.move = u.grab.move ?? null
    }
    if (u.grab || (u.roots && this.predicted?.released)) {
      this.predicted = null
    }
    if (u.frames) {
      this.frames = new Map(
        (u.frames.frames ?? []).map((f) => [f.root ?? 0, f]),
      )
    }

    // Take the layers, keeping each texture whose size holds.
    if (u.layers) {
      this.layers = u.layers.layers ?? []
      const textures = new Map<number, SceneTexture>()
      for (const l of this.layers) {
        const width = l.textureWidth ?? 0
        const height = l.textureHeight ?? 0
        const kept = this.textures.get(l.id ?? 0)
        textures.set(
          l.id ?? 0,
          kept && kept.width === width && kept.height === height
            ? kept
            : {
                width,
                height,
                pixels: new Uint8Array(width * height * 4),
                dirty: null,
              },
        )
      }
      this.textures = textures
    }

    // Copy each patch's rows into its texture.
    for (const p of u.patches ?? []) {
      const t = this.textures.get(p.layer ?? 0)
      const r = {
        x: p.x ?? 0,
        y: p.y ?? 0,
        width: p.width ?? 0,
        height: p.height ?? 0,
      }
      const rgba = p.rgba
      if (
        !t ||
        !rgba ||
        r.x + r.width > t.width ||
        r.y + r.height > t.height ||
        rgba.length !== r.width * r.height * 4
      ) {
        continue
      }
      const row = r.width * 4
      for (let y = 0; y < r.height; y++) {
        t.pixels.set(
          rgba.subarray(y * row, (y + 1) * row),
          ((r.y + y) * t.width + r.x) * 4,
        )
      }
      t.dirty?.push(r)
    }
  }

  // point takes the viewer's pointer at the screen pixel x, y, the position
  // it sends the compositor, and reports whether the scene changed: whether
  // a window moves with it.
  point(x: number, y: number): boolean {
    // Convert to units, keeping the pointer on the screen as the compositor
    // does.
    const { width, height, scale } = this.screen
    this.pointer = {
      x: Math.max(Math.min(Math.floor(x / scale), width - 1), 0),
      y: Math.max(Math.min(Math.floor(y / scale), height - 1), 0),
    }
    return this.moving() !== null
  }

  // press takes a first button press at the pointer, and starts moving the
  // window whose title bar is under it, as the compositor does on the same
  // press, so the window follows the next move without waiting for the
  // compositor. It reports whether a move started.
  press(): boolean {
    // Find a title bar under the pointer of a window that can move.
    const p = this.pointer
    const part = p && this.frameAt(p)
    if (!p || part?.kind !== 'title' || part.frame.maximized) return false

    // Move its root from where it shows.
    const id = part.frame.root ?? 0
    const at = this.rootAt(id)
    this.predicted = {
      move: {
        root: id,
        pressX: p.x,
        pressY: p.y,
        x: at.x,
        y: at.y,
        minY: part.frame.minY ?? 0,
      },
      released: false,
    }
    return true
  }

  // release takes the release of every button. A predicted move holds its
  // place until the compositor's roots or grab show where the window went.
  release() {
    if (this.predicted) this.predicted.released = true
  }

  // settle ends a released predicted move the compositor never confirmed,
  // and reports whether the scene changed.
  settle(): boolean {
    if (!this.predicted?.released) return false
    this.predicted = null
    return true
  }

  // moving returns the move in progress: the compositor's, else the one the
  // viewer predicted, or null.
  private moving(): WasiVmSceneMove | null {
    return this.move ?? this.predicted?.move ?? null
  }

  // frameAt returns the frame part at the point p in units, or null when a
  // window's surface or nothing framed is there. Roots are checked from the
  // top down, each by its layers and then its frame, as the compositor does.
  frameAt(p: ScenePoint): FramePart | null {
    // Walk the roots from the top, by their topmost layers.
    const seen = new Set<number>()
    for (let i = this.layers.length - 1; i >= 0; i--) {
      const id = this.layers[i]?.root ?? 0
      if (seen.has(id)) continue
      seen.add(id)
      const root = this.rootAt(id)

      // A layer of the root under the point takes it.
      const covered = this.layers.some(
        (l) =>
          (l.root ?? 0) === id &&
          inRect(p, {
            x: root.x + (l.x ?? 0),
            y: root.y + (l.y ?? 0),
            width: l.width ?? 0,
            height: l.height ?? 0,
          }),
      )
      if (covered) return null

      // Else the root's frame might cover the point.
      const f = this.frames.get(id)
      const part = f && framePartAt(f, { x: p.x - root.x, y: p.y - root.y })
      if (part) return part
    }
    return null
  }

  // cursorAt returns the CSS cursor for the pointer: a resize cursor on a
  // frame's band, else the default cursor.
  cursorAt(): string {
    const part = this.pointer && this.frameAt(this.pointer)
    return part?.kind === 'band' ? resizeCursor(part.edges) : ''
  }

  // rootAt returns the position of the root id: where the move puts it
  // under the viewer's pointer while it moves, else where the last roots
  // put it. The compositor places a moving root by the same rule, so the
  // roots catch up with the prediction rather than correct it.
  private rootAt(id: number): ScenePoint {
    const m = this.moving()
    const p = this.pointer
    if (m && p && m.root === id) {
      return {
        x: (m.x ?? 0) + p.x - (m.pressX ?? 0),
        y: Math.max((m.y ?? 0) + p.y - (m.pressY ?? 0), m.minY ?? 0),
      }
    }
    return this.roots.get(id) ?? { x: 0, y: 0 }
  }

  // drawList returns the layers bottom to top with their textures, placed at
  // their roots' positions, each framed root's edge and title bar below its
  // first layer. The edge is a unit wider than the frame on each side, so the
  // window covers all but its outline. A maximized window has no edge. The
  // frame's layers take the negated root id. The array is fresh. The textures
  // are the scene's.
  drawList(): SceneLayer[] {
    // Walk the layers, keeping each title bar painted.
    const paint = this.paint
    const out: SceneLayer[] = []
    const painted = new Map<number, FrameTexture>()
    for (const l of this.layers) {
      // Draw the root's edge and title bar first.
      const id = l.root ?? 0
      const root = this.rootAt(id)
      const f = this.frames.get(id)
      if (f && paint && !painted.has(id)) {
        const ft = this.frameTexture(paint, f)
        painted.set(id, ft)
        if (f.border) {
          this.edge ??= paint.edge()
          out.push({
            id: -id,
            x: root.x + (f.x ?? 0) - 1,
            y: root.y + (f.y ?? 0) - 1,
            width: (f.width ?? 0) + 2,
            height: (f.height ?? 0) + 2,
            opaque: true,
            texture: this.edge,
          })
        }
        out.push({
          id: -id,
          x: root.x + (f.x ?? 0),
          y: root.y + (f.y ?? 0),
          width: f.width ?? 0,
          height: f.titleHeight ?? 0,
          opaque: true,
          texture: ft.texture,
        })
      }

      // Then draw the layer.
      const texture = this.textures.get(l.id ?? 0)
      if (!texture || !texture.width || !texture.height) continue
      out.push({
        id: l.id ?? 0,
        x: root.x + (l.x ?? 0),
        y: root.y + (l.y ?? 0),
        width: l.width ?? 0,
        height: l.height ?? 0,
        opaque: l.opaque ?? false,
        texture,
      })
    }
    this.frameTextures = painted
    return out
  }

  // frameTexture returns the title bar of the frame f, painting it with
  // paint again only when what it shows changed.
  private frameTexture(paint: FramePainter, f: WasiVmSceneFrame): FrameTexture {
    const key = JSON.stringify([
      f.title,
      f.active,
      f.maximized,
      f.width,
      f.titleHeight,
      f.buttons,
      this.screen.scale,
    ])
    const kept = this.frameTextures.get(f.root ?? 0)
    if (kept?.key === key) return kept
    return { key, texture: paint.title(f, this.screen.scale) }
  }

  // allDirty marks every texture changed, so a renderer that lost its copies
  // uploads them all again.
  allDirty() {
    for (const t of this.textures.values()) {
      t.dirty = null
    }
  }
}

// inRect reports whether the point p is in the rectangle r.
function inRect(p: ScenePoint, r: SceneRect): boolean {
  return p.x >= r.x && p.x < r.x + r.width && p.y >= r.y && p.y < r.y + r.height
}

// framePartAt returns the part of the frame f at the point p relative to its
// root, or null: the band's edges outside the frame, a button, or the title
// bar. The window's content below the title bar is not the frame's.
function framePartAt(f: WasiVmSceneFrame, p: ScenePoint): FramePart | null {
  // Check the point is on the frame or its band, above the content.
  const r = {
    x: f.x ?? 0,
    y: f.y ?? 0,
    width: f.width ?? 0,
    height: f.height ?? 0,
  }
  const b = f.border ?? 0
  const band = {
    x: r.x - b,
    y: r.y - b,
    width: r.width + 2 * b,
    height: r.height + 2 * b,
  }
  const content = {
    x: r.x,
    y: r.y + (f.titleHeight ?? 0),
    width: r.width,
    height: r.height - (f.titleHeight ?? 0),
  }
  if (!inRect(p, band) || inRect(p, content)) return null

  // Find the band's edges, then a button.
  let edges = 0
  if (p.x < r.x) edges |= edgeLeft
  if (p.x >= r.x + r.width) edges |= edgeRight
  if (p.y < r.y) edges |= edgeTop
  if (p.y >= r.y + r.height) edges |= edgeBottom
  if (edges) return { kind: 'band', frame: f, edges }
  for (const btn of f.buttons ?? []) {
    const br = {
      x: btn.x ?? 0,
      y: btn.y ?? 0,
      width: btn.width ?? 0,
      height: btn.height ?? 0,
    }
    if (inRect(p, br)) {
      return {
        kind: 'button',
        frame: f,
        button: btn.kind ?? WasiVmSceneFrameButtonKind.UNKNOWN,
      }
    }
  }
  return { kind: 'title', frame: f }
}

// resizeCursor returns the CSS cursor that resizes by edges.
function resizeCursor(edges: number): string {
  const v = edges & (edgeTop | edgeBottom)
  const h = edges & (edgeLeft | edgeRight)
  if (v && h) {
    return (v === edgeTop) === (h === edgeLeft) ? 'nwse-resize' : 'nesw-resize'
  }
  return v ? 'ns-resize' : 'ew-resize'
}

// SceneRenderer draws scenes onto one canvas.
export interface SceneRenderer {
  // draw sizes the canvas to the screen in pixels, uploads the areas of each
  // texture changed since the last draw, and draws the layers bottom to top
  // over the background.
  draw(scene: Scene): void
  // destroy releases the renderer's textures and its device.
  destroy(): void
}

// takeDirty returns the areas of t changed since the last call, or null when
// all of it changed, and clears them.
export function takeDirty(t: SceneTexture): SceneRect[] | null {
  const dirty = t.dirty
  t.dirty = []
  return dirty
}
