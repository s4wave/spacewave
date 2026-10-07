import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { JsModuleKind } from '@go/github.com/s4wave/spacewave/bldr/plugin/compiler/js/compiler.pb.js'
import type { ValidatePluginRepositoryResponse } from '@s4wave/sdk/space/space.pb.js'

import { PluginRepositorySheet } from './PluginRepositorySheet.js'

const commit = 'aaaaaaa1111111111111111111111111111111111'

// accepted is the validation of a JavaScript plugin with pinned dependencies.
const accepted: ValidatePluginRepositoryResponse = {
  commit,
  sourceKey: 'plugin-repos/acme/colors/workdir',
  validation: {
    plugins: [
      {
        manifestId: 'acme-colors',
        description: 'Color swatches',
        modules: [
          { kind: JsModuleKind.BACKEND, path: './backend.ts' },
          { kind: JsModuleKind.FRONTEND, path: './frontend.tsx' },
        ],
      },
    ],
    dependencies: [
      { name: '@acme/util', version: '2.0.0' },
      { name: 'left-pad', version: '1.3.0', direct: true },
    ],
  },
}

/** renderSheet renders the sheet for a review and returns its callbacks. */
function renderSheet(review: ValidatePluginRepositoryResponse, stale = false) {
  const onConfirm = vi.fn()
  const onClose = vi.fn()
  render(
    <PluginRepositorySheet
      repository="acme/colors"
      review={review}
      device="devices/laptop"
      stale={stale}
      busy={false}
      onConfirm={onConfirm}
      onClose={onClose}
    />,
  )
  return { onConfirm, onClose }
}

describe('PluginRepositorySheet', () => {
  afterEach(cleanup)

  it('shows what an accepted repository declares and builds on confirm', () => {
    const { onConfirm } = renderSheet(accepted)

    // The sheet names the repository, commit, plugin and modules.
    expect(screen.getByText('Review acme/colors')).toBeDefined()
    expect(
      screen.getByText('Commit aaaaaaa. Nothing from this repository has run.'),
    ).toBeDefined()
    expect(screen.getByText('acme-colors')).toBeDefined()
    expect(screen.getByText('Color swatches')).toBeDefined()
    expect(screen.getByText('Backend: ./backend.ts')).toBeDefined()
    expect(screen.getByText('Frontend: ./frontend.tsx')).toBeDefined()

    // It lists the direct dependencies, counts the rest, and names the target.
    expect(
      screen.getByText(
        'left-pad@1.3.0 and 1 more package, each pinned by its content hash.',
      ),
    ).toBeDefined()
    expect(
      screen.getByText(
        'Builds on devices/laptop into your developer Space, at this commit only.',
      ),
    ).toBeDefined()

    // Confirming hands the decision back to the page.
    fireEvent.click(screen.getByRole('button', { name: 'Build' }))
    expect(onConfirm).toHaveBeenCalledOnce()
  })

  it('shows each refusal reason and offers no build', () => {
    renderSheet({
      commit,
      validation: {
        plugins: [{ manifestId: 'acme-colors' }],
        refusals: [
          { reason: 'Plugin "acme-go" is a Go plugin.' },
          { reason: 'package.json has a postinstall script.' },
        ],
      },
    })

    // The reasons replace the plugin list, and only Cancel remains.
    expect(
      screen.getByText('Spacewave will not build this repository'),
    ).toBeDefined()
    expect(screen.getByText('Plugin "acme-go" is a Go plugin.')).toBeDefined()
    expect(
      screen.getByText('package.json has a postinstall script.'),
    ).toBeDefined()
    expect(screen.queryByText('acme-colors')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Build' })).toBeNull()
  })

  it('asks for a new review once the repository moves', () => {
    renderSheet(accepted, true)

    // A stale review cannot build.
    expect(
      screen.getByText(
        'The repository has moved to another commit. Review it again.',
      ),
    ).toBeDefined()
    expect(
      screen.getByRole('button', { name: 'Build' }).hasAttribute('disabled'),
    ).toBe(true)
  })
})
