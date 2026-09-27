import { expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { render } from 'vitest-browser-react'

import { MessageInput } from './MessageInput.js'

it('keeps Send touch-sized through rotation and compact for a mouse', async () => {
  // Measure the production composer at both phone widths and short landscape.
  const coarse = matchMedia('(pointer: coarse)').matches
  await render(<MessageInput onSend={vi.fn(async () => {})} />)
  for (const [width, height] of [
    [390, 844],
    [360, 780],
    [844, 390],
  ]) {
    await page.viewport(width, height)
    const send = page.getByRole('button', { name: 'Send message' }).element()
    const input = page.getByRole('textbox', { name: 'Message' }).element()
    const targetSize = coarse || width < 640 ? 44 : 36
    expect(send.getBoundingClientRect().width).toBe(targetSize)
    expect(send.getBoundingClientRect().height).toBe(targetSize)
    expect(input.getBoundingClientRect().height).toBe(targetSize)
    expect(send.getBoundingClientRect().right).toBeLessThanOrEqual(width)
    expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(width)
  }
})
