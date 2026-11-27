import { describe, expect, it } from 'vitest'

import { WasiVmSceneFrameButtonKind } from '@s4wave/sdk/vm/wasivm.pb.js'

import {
  Scene,
  edgeBottom,
  edgeRight,
  takeDirty,
  type FramePainter,
} from './display-scene.js'

// layer returns a layer of root 1 at the origin with a texture of w by h
// pixels covering w by h units.
function layer(id: number, w: number, h: number) {
  return {
    id,
    root: 1,
    width: w,
    height: h,
    textureWidth: w,
    textureHeight: h,
  }
}

describe('Scene', () => {
  it('places layers at their roots and keeps textures across moves', () => {
    // Show one 2x2 layer, filled by a full patch.
    const scene = new Scene()
    const full = new Uint8Array(16).fill(9)
    scene.apply({
      screen: { width: 10, height: 10, scale: 2, background: 0x112233ff },
      layers: { layers: [layer(5, 2, 2)] },
      roots: { roots: [{ id: 1, x: 3, y: 4 }] },
      patches: [{ layer: 5, x: 0, y: 0, width: 2, height: 2, rgba: full }],
    })
    const [first] = scene.drawList()
    expect(first).toMatchObject({ id: 5, x: 3, y: 4, width: 2, height: 2 })
    expect(scene.screen.scale).toBe(2)
    expect(takeDirty(first!.texture)).toBeNull()

    // Move the root: the texture and its pixels stay, nothing is dirty.
    scene.apply({ roots: { roots: [{ id: 1, x: 7, y: 1 }] } })
    const [moved] = scene.drawList()
    expect(moved).toMatchObject({ x: 7, y: 1 })
    expect(moved!.texture).toBe(first!.texture)
    expect(moved!.texture.pixels).toEqual(full)
    expect(takeDirty(moved!.texture)).toEqual([])

    // Patch one pixel: only its rectangle is dirty.
    scene.apply({
      patches: [
        {
          layer: 5,
          x: 1,
          y: 1,
          width: 1,
          height: 1,
          rgba: Uint8Array.of(1, 2, 3, 4),
        },
      ],
    })
    const t = scene.drawList()[0]!.texture
    expect(Array.from(t.pixels.subarray(12))).toEqual([1, 2, 3, 4])
    expect(takeDirty(t)).toEqual([{ x: 1, y: 1, width: 1, height: 1 }])
  })

  it('moves a grabbed root with the local pointer until the grab ends', () => {
    // Show two roots at scale 2, and a move of root 1 pressed at 10,10 when
    // it sat at 4,6, whose top stays at 5 or below.
    const scene = new Scene()
    const one = { ...layer(1, 2, 2), root: 1 }
    const two = { ...layer(2, 2, 2), root: 2 }
    scene.apply({
      screen: { width: 40, height: 30, scale: 2 },
      layers: { layers: [one, two] },
      roots: {
        roots: [
          { id: 1, x: 4, y: 6 },
          { id: 2, x: 20, y: 20 },
        ],
      },
    })
    const at = (id: number) => scene.drawList().find((l) => l.id === id)
    expect(scene.point(0, 0)).toBe(false)
    scene.apply({
      grab: { move: { root: 1, pressX: 10, pressY: 10, x: 4, y: 6, minY: 5 } },
    })

    // Moving the pointer, in screen pixels, moves root 1 by its travel in
    // units and leaves root 2.
    expect(scene.point(31, 25)).toBe(true)
    expect(at(1)).toMatchObject({ x: 9, y: 8 })
    expect(at(2)).toMatchObject({ x: 20, y: 20 })

    // Root 1's top stops at the minimum y coordinate, and the pointer stays
    // on the screen.
    scene.point(-50, 0)
    expect(at(1)).toMatchObject({ x: -6, y: 5 })

    // Roots that trail the pointer do not pull it back.
    scene.apply({ roots: { roots: [{ id: 1, x: 7, y: 7 }] } })
    expect(at(1)).toMatchObject({ x: -6, y: 5 })

    // When the grab ends, the roots place root 1 again.
    scene.apply({ grab: {}, roots: { roots: [{ id: 1, x: -6, y: 5 }] } })
    expect(scene.point(100, 100)).toBe(false)
    expect(at(1)).toMatchObject({ x: -6, y: 5 })
  })

  it('drops gone layers and replaces resized textures', () => {
    // Show two layers, then list only the second, resized.
    const scene = new Scene()
    scene.apply({
      layers: { layers: [layer(1, 2, 2), layer(2, 2, 2)] },
      roots: { roots: [{ id: 1 }] },
    })
    const before = scene.drawList()[1]!.texture
    takeDirty(before)
    scene.apply({ layers: { layers: [layer(2, 3, 1)] } })

    // The second layer remains with a new, wholly dirty texture.
    const after = scene.drawList()
    expect(after.map((l) => l.id)).toEqual([2])
    expect(after[0]!.texture).not.toBe(before)
    expect(after[0]!.texture).toMatchObject({
      width: 3,
      height: 1,
      dirty: null,
    })
  })

  it('ignores a patch outside its texture', () => {
    const scene = new Scene()
    scene.apply({
      layers: { layers: [layer(1, 2, 2)] },
      roots: { roots: [{ id: 1 }] },
      patches: [
        {
          layer: 1,
          x: 1,
          y: 1,
          width: 2,
          height: 1,
          rgba: new Uint8Array(8).fill(7),
        },
      ],
    })
    expect(scene.drawList()[0]!.texture.pixels.every((b) => b === 0)).toBe(true)
  })

  it('draws frames and moves a window from a title bar press', () => {
    // Show a 20x10 window at 5,30 under a 4 unit title bar, with a close
    // button at its right end, and a least top of 6.
    let paints = 0
    const texture = (width: number, height: number) => ({
      width,
      height,
      pixels: new Uint8Array(width * height * 4),
      dirty: null,
    })
    const paint: FramePainter = {
      title: (f, scale) => {
        paints++
        return texture((f.width ?? 0) * scale, (f.titleHeight ?? 0) * scale)
      },
      edge: () => texture(1, 1),
    }
    const scene = new Scene(paint)
    scene.apply({
      screen: { width: 100, height: 100, scale: 1 },
      layers: { layers: [layer(1, 20, 10)] },
      roots: { roots: [{ id: 1, x: 5, y: 30 }] },
      frames: {
        frames: [
          {
            root: 1,
            title: 'foot',
            active: true,
            x: 0,
            y: -4,
            width: 20,
            height: 14,
            titleHeight: 4,
            border: 2,
            minY: 6,
            buttons: [
              {
                kind: WasiVmSceneFrameButtonKind.CLOSE,
                x: 16,
                y: -4,
                width: 4,
                height: 4,
              },
            ],
          },
        ],
      },
    })

    // The edge and the title bar draw below the window, the edge a unit
    // outside the frame and the title bar at its top.
    const list = scene.drawList()
    expect(list.map((l) => l.id)).toEqual([-1, -1, 1])
    expect(list[0]).toMatchObject({ x: 4, y: 25, width: 22, height: 16 })
    expect(list[1]).toMatchObject({
      x: 5,
      y: 26,
      width: 20,
      height: 4,
      opaque: true,
    })

    // The pointer finds the button, the band and the title bar, and the
    // window's content is not the frame's.
    scene.point(22, 27)
    expect(scene.frameAt({ x: 22, y: 27 })).toMatchObject({
      kind: 'button',
      button: WasiVmSceneFrameButtonKind.CLOSE,
    })
    expect(scene.frameAt({ x: 26, y: 41 })).toMatchObject({
      kind: 'band',
      edges: edgeRight | edgeBottom,
    })
    expect(scene.frameAt({ x: 10, y: 35 })).toBeNull()
    scene.point(26, 41)
    expect(scene.cursorAt()).toBe('nwse-resize')

    // A press on a button starts no move.
    scene.point(22, 27)
    expect(scene.press()).toBe(false)

    // A press on the title bar starts a move, and the window follows the
    // pointer at once, its top held at the minimum y coordinate.
    scene.point(8, 27)
    expect(scene.press()).toBe(true)
    expect(scene.point(18, 37)).toBe(true)
    expect(scene.drawList()[2]).toMatchObject({ x: 15, y: 40 })
    scene.point(18, 0)
    expect(scene.drawList()[2]).toMatchObject({ x: 15, y: 6 })
    expect(scene.drawList()[1]).toMatchObject({ x: 15, y: 2 })

    // After the release the move holds until the roots show where the
    // window went, and the title bar is not painted again.
    scene.release()
    scene.apply({ layers: { layers: [layer(1, 20, 10)] } })
    expect(scene.drawList()[2]).toMatchObject({ x: 15, y: 6 })
    scene.apply({ roots: { roots: [{ id: 1, x: 15, y: 6 }] } })
    expect(scene.point(50, 50)).toBe(false)
    expect(scene.drawList()[2]).toMatchObject({ x: 15, y: 6 })
    expect(paints).toBe(1)

    // A released move the compositor never answers settles back.
    scene.point(20, 4)
    expect(scene.press()).toBe(true)
    scene.point(30, 10)
    scene.release()
    expect(scene.settle()).toBe(true)
    expect(scene.drawList()[2]).toMatchObject({ x: 15, y: 6 })
  })
})
