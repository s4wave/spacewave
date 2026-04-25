import { describe, expect, it, vi } from 'vitest'
import { createRef } from 'react'
import { render } from '@testing-library/react'

import {
  Turnstile,
  TURNSTILE_PROD_SITE_KEY,
  TURNSTILE_TEST_SITE_KEY,
  TURNSTILE_TEST_TOKEN,
  isTurnstileBypassed,
  type TurnstileInstance,
} from './turnstile.js'

describe('turnstile', () => {
  it('does not bypass the production key', () => {
    expect(isTurnstileBypassed(TURNSTILE_PROD_SITE_KEY)).toBe(false)
  })

  it('bypasses widget loading for the test key', async () => {
    const appendChild = vi.spyOn(document.head, 'appendChild')
    const ref = createRef<TurnstileInstance>()
    const { container } = render(
      <Turnstile ref={ref} siteKey={TURNSTILE_TEST_SITE_KEY} />,
    )

    expect(isTurnstileBypassed(TURNSTILE_TEST_SITE_KEY)).toBe(true)
    expect(container.firstChild).toBeNull()
    expect(appendChild).not.toHaveBeenCalled()
    expect(ref.current?.getResponse()).toBe(TURNSTILE_TEST_TOKEN)
    await expect(ref.current?.getResponsePromise()).resolves.toBe(
      TURNSTILE_TEST_TOKEN,
    )

    appendChild.mockRestore()
  })
})
