import React from 'react'
import { expect, test } from 'vitest'
import { render } from 'vitest-browser-react'

import {
  Codec,
  type FrameEvent,
  type Input,
} from '@s4wave/core/frametrack/frametrack.pb.js'

import { FrameView } from './FrameView.js'
import type { FrameSource } from './play-frames.js'

/** waitForRole waits for the player's next accessible state. */
function waitForRole(canvas: HTMLCanvasElement, role: string): Promise<void> {
  if (canvas.getAttribute('role') === role) return Promise.resolve()
  return new Promise((resolve) => {
    const observer = new MutationObserver(() => {
      if (canvas.getAttribute('role') !== role) return
      observer.disconnect()
      resolve()
    })
    observer.observe(canvas, { attributeFilter: ['role'] })
  })
}

/** jpegPicture encodes a picture through the same browser bitmap path. */
async function jpegPicture(): Promise<FrameEvent> {
  const canvas = new OffscreenCanvas(160, 90)
  canvas.getContext('2d')!.fillRect(0, 0, 160, 90)
  const bitmap = await canvas.convertToBlob({ type: 'image/jpeg' })
  return {
    state: {
      case: 'frame',
      value: {
        header: {
          codec: Codec.JPEG,
          width: 160,
          height: 90,
          groupSequence: 1n,
        },
        frame: {
          keyframe: true,
          data: new Uint8Array(await bitmap.arrayBuffer()),
        },
      },
    },
  }
}

test('only a live display forwards pointer and key input', async () => {
  // Hold the first picture until both canvases are mounted and track input.
  const picture = await jpegPicture()
  const releasePicture = Promise.withResolvers<void>()
  const sent = Promise.withResolvers<Input[]>()
  const display: FrameSource = {
    kind: 'control',
    async *open(input) {
      // A connection with no picture must offer no input.
      yield { state: { case: 'opened', value: {} } }
      await releasePicture.promise
      yield picture

      // Read the DOM input in the order the display RPC would consume it.
      const events: Input[] = []
      for await (const event of input) {
        events.push(event)
        if (events.length === 3) sent.resolve(events.slice())
      }
    },
  }
  const camera: FrameSource = {
    kind: 'view',
    async *open(signal) {
      yield picture
      await new Promise<void>((resolve) =>
        signal.addEventListener('abort', () => resolve(), { once: true }),
      )
    },
  }

  // Let the real player size and paint each canvas inside its pane.
  const view = await render(
    <>
      <div className="flex h-50 w-80">
        <FrameView label="Test display" source={display} />
      </div>
      <div className="flex h-50 w-80">
        <FrameView label="Test camera" source={camera} />
      </div>
    </>,
  )
  const displayCanvas = view.container.querySelector<HTMLCanvasElement>(
    '[aria-label="Test display"]',
  )!
  const cameraCanvas = view.container.querySelector<HTMLCanvasElement>(
    '[aria-label="Test camera"]',
  )!
  expect(displayCanvas.getAttribute('role')).toBe('img')
  expect(displayCanvas.hasAttribute('tabindex')).toBe(false)

  // Only the display becomes a keyboard and pointer target after painting.
  releasePicture.resolve()
  await waitForRole(displayCanvas, 'application')
  expect(displayCanvas.tabIndex).toBe(0)
  expect(cameraCanvas.getAttribute('role')).toBe('img')
  expect(cameraCanvas.hasAttribute('tabindex')).toBe(false)

  // Move within the letterboxed picture and forward a full key transition.
  const bounds = displayCanvas.getBoundingClientRect()
  displayCanvas.dispatchEvent(
    new PointerEvent('pointermove', {
      bubbles: true,
      clientX: bounds.left + bounds.width / 2,
      clientY: bounds.top + bounds.height / 2,
    }),
  )
  displayCanvas.focus()
  for (const type of ['keydown', 'keyup']) {
    displayCanvas.dispatchEvent(
      new KeyboardEvent(type, { bubbles: true, code: 'KeyA', key: 'a' }),
    )
  }
  expect(await sent.promise).toEqual([
    { event: { case: 'pointer', value: { x: 0.5, y: 0.5, buttons: 0 } } },
    {
      event: {
        case: 'key',
        value: { code: 'KeyA', key: 'a', pressed: true, repeat: false },
      },
    },
    {
      event: {
        case: 'key',
        value: { code: 'KeyA', key: 'a', pressed: false, repeat: false },
      },
    },
  ])

  // The stall deadline hides the input target while holding the last picture.
  await waitForRole(displayCanvas, 'img')
  expect(displayCanvas.hasAttribute('tabindex')).toBe(false)
}, 10000)
