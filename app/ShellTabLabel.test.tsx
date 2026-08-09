import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ShellTabLabel } from './ShellTabLabel.js'

const shellTabs = vi.hoisted(() => ({
  renamingTabId: 'tab-1' as string | null,
  stopRenaming: vi.fn(),
  updateTabName: vi.fn(),
}))

vi.mock('./ShellTabContext.js', () => ({
  useShellTabs: () => shellTabs,
}))

describe('ShellTabLabel', () => {
  beforeEach(() => {
    shellTabs.renamingTabId = 'tab-1'
    shellTabs.stopRenaming.mockReset()
    shellTabs.updateTabName.mockReset()
  })

  afterEach(() => cleanup())

  it('opens a requested rename with the current name focused and selected', () => {
    render(
      <ShellTabLabel
        tab={{ id: 'tab-1', name: 'Settings', path: '/settings' }}
      />,
    )

    const input = screen.getByRole('textbox') as HTMLInputElement
    expect(input.value).toBe('Settings')
    expect(document.activeElement).toBe(input)
    expect(input.selectionStart).toBe(0)
    expect(input.selectionEnd).toBe('Settings'.length)

    fireEvent.change(input, { target: { value: '  Account  ' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(shellTabs.updateTabName).toHaveBeenCalledWith('tab-1', 'Account')
    expect(shellTabs.stopRenaming).toHaveBeenCalledWith('tab-1')
  })
})
