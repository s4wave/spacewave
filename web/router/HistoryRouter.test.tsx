import { useCallback, useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { resolvePath, type To, useNavigate } from '@s4wave/web/router/router.js'

import { HistoryRouter, useHistory } from './HistoryRouter.js'

vi.mock('@aptre/bldr', () => ({
  cleanPath: (path: string) => path.replace(/\/{2,}/g, '/'),
  joinPath: (parts: string[], absolute: boolean) =>
    `${absolute ? '/' : ''}${parts.filter(Boolean).join('/')}`,
  splitPath: (path: string) => ({
    pathParts: path.split('/').filter(Boolean),
    isAbsolute: path.startsWith('/'),
  }),
}))

function HistoryControls() {
  const navigate = useNavigate()
  const history = useHistory()

  return (
    <>
      <button onClick={() => navigate({ path: 'video.mp4' })} type="button">
        Open video
      </button>
      <button onClick={() => history?.goBack()} type="button">
        Back
      </button>
    </>
  )
}

function HistoryHarness({ onNavigate }: { onNavigate: (to: To) => void }) {
  const [path, setPath] = useState('/')
  const handleNavigate = useCallback(
    (to: To) => {
      onNavigate(to)
      setPath((current) => resolvePath(current, to))
    },
    [onNavigate],
  )

  return (
    <HistoryRouter path={path} onNavigate={handleNavigate}>
      <HistoryControls />
    </HistoryRouter>
  )
}

describe('HistoryRouter', () => {
  it('marks Back targets as local history navigation', () => {
    const onNavigate = vi.fn()
    render(<HistoryHarness onNavigate={onNavigate} />)

    fireEvent.click(screen.getByRole('button', { name: 'Open video' }))
    fireEvent.click(screen.getByRole('button', { name: 'Back' }))

    expect(onNavigate).toHaveBeenLastCalledWith({
      path: '/',
      history: 'local',
    })
  })
})
