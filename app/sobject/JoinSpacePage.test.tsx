import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { JoinSpacePage } from './JoinSpacePage.js'

const mockNavigate = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => ({ code: 'bearer:invite-1' }),
}))

vi.mock('./JoinSpaceDialog.js', () => ({
  JoinSpaceDialog: (props: {
    onAccepted: (sharedObjectId: string) => void
  }) => (
    <button onClick={() => props.onAccepted('space-1')}>
      Open the shared Space
    </button>
  ),
}))

describe('JoinSpacePage', () => {
  it('opens the accepted shared Space returned by the join result', () => {
    render(<JoinSpacePage />)

    fireEvent.click(
      screen.getByRole('button', { name: 'Open the shared Space' }),
    )

    expect(mockNavigate).toHaveBeenCalledWith({
      path: '../so/space-1',
      replace: true,
    })
  })
})
