import { describe, expect, it, vi } from 'vitest'
import { page, userEvent } from 'vitest/browser'
import { render } from 'vitest-browser-react'

import { CommandPalette } from './CommandPalette.js'

const palette: { open?: () => void } = vi.hoisted(() => ({}))

vi.mock('./CommandContext.js', () => ({
  useCommands: () => [],
  useInvokeCommand: () => () => {},
  useCommandContext: () => ({
    getSubItems: async () => [],
    registerOpenCommand: () => () => {},
  }),
}))

vi.mock('./useCommand.js', () => ({
  useCommand: (options: { commandId: string; handler: () => void }) => {
    if (options.commandId === 'spacewave.view.palette') {
      palette.open = options.handler
    }
  },
}))

describe('CommandPalette browser input editing', () => {
  it('allows native select-all then Delete to clear the command search input', async () => {
    await render(<CommandPalette />)

    palette.open?.()

    const input = page.getByRole('combobox')
    await userEvent.type(input, 'browser query')
    await expect.element(input).toHaveValue('browser query')

    await userEvent.keyboard('{ControlOrMeta>}a{/ControlOrMeta}{Delete}')

    await expect.element(input).toHaveValue('')
  })
})
