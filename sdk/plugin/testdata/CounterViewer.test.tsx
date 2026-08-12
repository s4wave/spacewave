import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () => ({ value: {} }),
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: () => ({ value: 7n, error: null }),
}))

vi.mock('@s4wave/web/object/object.js', () => ({
  getObjectKey: () => 'counter/main',
}))

import { CounterViewer } from './CounterViewer.js'

describe('CounterViewer', () => {
  it('renders the value read through its typed Resource hook', () => {
    render(<CounterViewer objectInfo={{} as never} worldState={{} as never} />)

    expect(screen.getByLabelText('Counter value').textContent).toBe('7')
  })
})
