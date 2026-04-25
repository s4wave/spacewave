/**
 * Simple E2E test to verify vitest browser mode works.
 * This test doesn't depend on the SDK, just tests basic browser functionality.
 */
import { describe, it, expect } from 'vitest'
import { page } from 'vitest/browser'
import { render, cleanup } from 'vitest-browser-react'

function SimpleComponent({ text }: { text: string }) {
  return <div data-testid="simple">{text}</div>
}

describe('Simple Browser Test', () => {
  it('renders a component', async () => {
    await render(<SimpleComponent text="Hello World" />)

    await expect
      .element(page.getByTestId('simple'))
      .toHaveTextContent('Hello World')

    await cleanup()
  })
})
