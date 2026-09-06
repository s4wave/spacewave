import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { demoCode } from './demo-code.js'
import { DemoPreBlock } from './DemoPreBlock.js'

describe('landing demo code', () => {
  afterEach(cleanup)

  it('renders each built-in example with highlights immediately and copies its source', () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.spyOn(navigator.clipboard, 'writeText').mockImplementation(writeText)

    for (const { lang, code } of Object.values(demoCode)) {
      const { container, unmount } = render(
        <DemoPreBlock>
          <code className={`lang-${lang}`}>{code + '\n'}</code>
        </DemoPreBlock>,
      )
      expect(container.querySelector('pre.shiki code')?.textContent).toBe(code)
      expect(container.querySelector('span[style]')).toBeTruthy()
      fireEvent.click(screen.getByTitle('Copy code'))
      expect(writeText).toHaveBeenLastCalledWith(code)
      unmount()
    }
    vi.restoreAllMocks()
  })

  it('renders edited code as text without retaining the original highlights', () => {
    const { code, lang } = demoCode.plugin
    const { container, rerender } = render(
      <DemoPreBlock>
        <code className={`lang-${lang}`}>{code}</code>
      </DemoPreBlock>,
    )
    const edited = '<img src=x onerror="alert(1)">'
    rerender(
      <DemoPreBlock>
        <code className={`lang-${lang}`}>{edited}</code>
      </DemoPreBlock>,
    )
    expect(container.querySelector('pre code')?.textContent).toBe(edited)
    expect(container.querySelector('.shiki')).toBeNull()
    expect(container.querySelector('img')).toBeNull()
  })
})
