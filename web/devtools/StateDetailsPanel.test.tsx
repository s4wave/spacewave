import React from 'react'
import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'

import { atom } from '@s4wave/web/state/index.js'

import { StateDetailsPanel } from './StateDetailsPanel.js'
import type { StateInspectorEntry } from './useStateInspectorEntries.js'

describe('StateDetailsPanel', () => {
  it('renders the selected atom value', () => {
    const entry: StateInspectorEntry = {
      kind: 'legacy',
      id: 'local:test',
      label: 'test',
      scope: 'local',
      atom: atom({ enabled: true, nested: { count: 1 } }),
    }

    render(<StateDetailsPanel entry={entry} />)

    expect(screen.getByText('test')).toBeDefined()
    expect(screen.getByText('local')).toBeDefined()
    expect(screen.getByText(/"enabled": true/)).toBeDefined()
  })
})
