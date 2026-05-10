import { afterEach, describe, expect, it } from 'vitest'
import { render, cleanup } from 'vitest-browser-react'

import '@s4wave/web/style/app.css'

import { AboutDialog } from './AboutDialog.js'

function AboutDialogInAppFrame() {
  return (
    <>
      <div style={{ minHeight: '100vh' }}>App frame</div>
      <AboutDialog open={true} onOpenChange={() => {}} />
    </>
  )
}

function getDialogContent(): HTMLElement {
  const el = document.querySelector('[data-slot="dialog-content"]')
  if (!(el instanceof HTMLElement)) {
    throw new Error('About dialog content was not rendered')
  }
  return el
}

interface DialogBounds {
  left: number
  top: number
  right: number
  bottom: number
  centerDeltaX: number
  centerDeltaY: number
}

function getDialogBounds(): DialogBounds {
  const rect = getDialogContent().getBoundingClientRect()
  const centerX = rect.left + rect.width / 2
  const centerY = rect.top + rect.height / 2
  return {
    left: Math.round(rect.left),
    top: Math.round(rect.top),
    right: Math.round(rect.right),
    bottom: Math.round(rect.bottom),
    centerDeltaX: Math.round(Math.abs(centerX - window.innerWidth / 2)),
    centerDeltaY: Math.round(Math.abs(centerY - window.innerHeight / 2)),
  }
}

describe('AboutDialog layout', () => {
  afterEach(() => {
    void cleanup()
  })

  it('centers the dialog content inside the viewport', async () => {
    await render(<AboutDialogInAppFrame />)

    await expect.poll(getDialogBounds).toSatisfy((rect: DialogBounds) => {
      return (
        rect.top >= 0 &&
        rect.left >= 0 &&
        rect.bottom <= window.innerHeight &&
        rect.right <= window.innerWidth &&
        rect.centerDeltaX <= 2 &&
        rect.centerDeltaY <= 2
      )
    })
  })
})
