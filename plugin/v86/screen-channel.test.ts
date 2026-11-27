import { describe, expect, it, vi } from 'vitest'

import {
  ScreenRelay,
  type ScreenEmulator,
  type ScreenFrame,
} from './screen-channel.js'

// FakeChannel records the frames a relay posts and delivers viewer input.
class FakeChannel {
  posted: ScreenFrame[] = []
  onmessage: ((ev: { data: ScreenFrame }) => void) | null = null

  postMessage(frame: ScreenFrame) {
    this.posted.push(frame)
  }

  send(frame: ScreenFrame) {
    this.onmessage?.({ data: frame })
  }
}

function fakeEmulator(): ScreenEmulator {
  return {
    screen_fill_buffer: vi.fn(),
    screen_redraw: vi.fn(),
    keyboard_send_event: vi.fn(),
    keyboard_release_all: vi.fn(),
    keyboard_send_scancodes: vi.fn(() => Promise.resolve()),
  }
}

describe('ScreenRelay', () => {
  it('batches the calls of one emulator slice into one frame', async () => {
    // Make two calls in one slice.
    const channel = new FakeChannel()
    const relay = new ScreenRelay(channel as never)
    relay.put_char(0, 0, 65, 0, 0, 0xffffff)
    relay.update_cursor(0, 1)
    await Promise.resolve()

    // Expect one frame carrying both.
    expect(channel.posted).toEqual([
      {
        dir: 'out',
        calls: [
          ['put_char', 0, 0, 65, 0, 0, 0xffffff],
          ['update_cursor', 0, 1],
        ],
      },
    ])
  })

  it('sends the font plane only when it changed and replays it for a new viewer', async () => {
    // Set the font, then change only its page.
    const channel = new FakeChannel()
    const relay = new ScreenRelay(channel as never)
    const emulator = fakeEmulator()
    relay.attach(emulator)
    const plane = new Uint8Array([1, 2, 3])
    relay.set_mode(false)
    relay.set_font_bitmap(16, true, false, false, plane, true)
    relay.set_font_bitmap(16, true, false, false, plane, false)
    await Promise.resolve()

    // Expect the plane in the first font call only.
    const [first] = channel.posted
    expect(first?.dir === 'out' && first.calls[1]?.[5]).toEqual(plane)
    expect(first?.dir === 'out' && first.calls[2]?.[5]).toBeNull()

    // Expect a hello to replay the mode and the font with its plane, then
    // redraw.
    channel.posted.length = 0
    channel.send({ dir: 'in', op: { kind: 'hello' } })
    await Promise.resolve()
    expect(channel.posted).toEqual([
      {
        dir: 'out',
        calls: [
          ['set_mode', false],
          ['set_font_bitmap', 16, true, false, false, plane, true],
        ],
      },
    ])
    expect(emulator.screen_redraw).toHaveBeenCalledOnce()
  })

  it('answers a fill with a frame even when nothing changed', async () => {
    // Request a fill before the emulator attaches, then after the attachment.
    const channel = new FakeChannel()
    const relay = new ScreenRelay(channel as never)
    channel.send({ dir: 'in', op: { kind: 'fill' } })
    await Promise.resolve()
    const emulator = fakeEmulator()
    relay.attach(emulator)
    channel.send({ dir: 'in', op: { kind: 'fill' } })
    await Promise.resolve()

    // Expect two empty answers and one emulator fill.
    expect(channel.posted).toEqual([
      { dir: 'out', calls: [] },
      { dir: 'out', calls: [] },
    ])
    expect(emulator.screen_fill_buffer).toHaveBeenCalledOnce()
  })

  it('copies the changed framebuffer rows out of the layer', async () => {
    // Draw rows 1 and 2 of a 2x4 buffer.
    const channel = new FakeChannel()
    const relay = new ScreenRelay(channel as never)
    const data = new Uint8ClampedArray(2 * 4 * 4).map((_, i) => i)
    relay.update_buffer([
      {
        image_data: { data, width: 2, height: 4 } as ImageData,
        screen_x: 0,
        screen_y: 1,
        buffer_x: 0,
        buffer_y: 1,
        buffer_width: 2,
        buffer_height: 2,
      },
    ])
    await Promise.resolve()

    // Expect only those rows, owned by the frame.
    const [frame] = channel.posted
    const call = frame?.dir === 'out' ? frame.calls[0] : undefined
    expect(call?.[0]).toBe('update_buffer')
    const rows = call?.[0] === 'update_buffer' ? call[1][0] : undefined
    expect(rows?.data).toEqual(data.slice(8, 24))
    expect(rows?.data.buffer).not.toBe(data.buffer)
    expect(rows?.screen_y).toBe(1)
  })

  it('relays viewer keys to the emulator', () => {
    // Press a key, release all keys, send Ctrl+Alt+Delete.
    const channel = new FakeChannel()
    const relay = new ScreenRelay(channel as never)
    const emulator = fakeEmulator()
    relay.attach(emulator)
    const key = {
      code: 'KeyA',
      key: 'a',
      keyCode: 65,
      altKey: false,
      shiftKey: false,
      ctrlKey: false,
      repeat: false,
    }
    channel.send({ dir: 'in', op: { kind: 'key', key, down: true } })
    channel.send({ dir: 'in', op: { kind: 'release' } })
    channel.send({ dir: 'in', op: { kind: 'ctrl-alt-del' } })

    // Expect each to reach the emulator.
    expect(emulator.keyboard_send_event).toHaveBeenCalledWith(key, true)
    expect(emulator.keyboard_release_all).toHaveBeenCalledOnce()
    expect(emulator.keyboard_send_scancodes).toHaveBeenCalledOnce()
  })
})
