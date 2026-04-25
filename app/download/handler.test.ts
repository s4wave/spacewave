import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { useDownloadDesktopApp } from './handler.js'

const mockNavigate = vi.fn()

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

describe('useDownloadDesktopApp', () => {
  afterEach(() => {
    mockNavigate.mockReset()
  })

  it('navigates to /download when invoked', () => {
    const { result } = renderHook(() => useDownloadDesktopApp())

    result.current()

    expect(mockNavigate).toHaveBeenCalledWith({ path: '/download' })
  })
})
