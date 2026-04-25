import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render } from '@testing-library/react'

import { NavigatePath } from './NavigatePath.js'
import { RouterProvider } from './router.js'

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('NavigatePath', () => {
  it('navigates to the requested path on mount', () => {
    const onNavigate = vi.fn()

    render(
      <RouterProvider path="/" onNavigate={onNavigate}>
        <NavigatePath to="verify-email" replace />
      </RouterProvider>,
    )

    expect(onNavigate).toHaveBeenCalledTimes(1)
    expect(onNavigate).toHaveBeenCalledWith({
      path: 'verify-email',
      replace: true,
    })
  })

  it('does not redispatch the same navigation when the navigate callback changes', () => {
    const onNavigate = vi.fn()
    const nextOnNavigate = vi.fn()

    const view = render(
      <RouterProvider path="/" onNavigate={onNavigate}>
        <NavigatePath to="verify-email" replace />
      </RouterProvider>,
    )

    expect(onNavigate).toHaveBeenCalledTimes(1)

    view.rerender(
      <RouterProvider path="/" onNavigate={nextOnNavigate}>
        <NavigatePath to="verify-email" replace />
      </RouterProvider>,
    )

    expect(nextOnNavigate).not.toHaveBeenCalled()
  })
})
