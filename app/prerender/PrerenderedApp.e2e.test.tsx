import { describe, it, expect, beforeEach } from 'vitest'
import { page } from 'vitest/browser'
import { render, cleanup } from 'vitest-browser-react'

import { PrerenderedApp } from './PrerenderedApp.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    overflow: 'hidden',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    height: `calc(${themeLayout['--spacing']} * 192)`,
    width: `calc(${themeLayout['--spacing']} * 256)`,
  },
})

function TestContainer({ children }: { children: React.ReactNode }) {
  return <div sx={stylexStyles.rule1}>{children}</div>
}

describe('PrerenderedApp E2E', () => {
  beforeEach(() => {
    void cleanup()
    localStorage.clear()
    window.location.hash = ''
  })

  it('renders the landing page with SPACEWAVE title', async () => {
    await render(
      <TestContainer>
        <PrerenderedApp />
      </TestContainer>,
    )

    await expect
      .poll(
        () => {
          const el = page.getByText('[SPACEWAVE]').element()
          return el
        },
        { timeout: 5000 },
      )
      .not.toBeNull()
  })

  it('renders navigation links', async () => {
    await render(
      <TestContainer>
        <PrerenderedApp />
      </TestContainer>,
    )

    await expect
      .poll(
        () => {
          const el = page.getByText('Docs', { exact: true }).element()
          return el
        },
        { timeout: 5000 },
      )
      .not.toBeNull()
  })

  it('renders Get Started section with quickstart options', async () => {
    await render(
      <TestContainer>
        <PrerenderedApp />
      </TestContainer>,
    )

    await expect
      .poll(
        () => {
          const el = page
            .getByText('Where would you like to start?', { exact: true })
            .element()
          return el
        },
        { timeout: 5000 },
      )
      .not.toBeNull()
  })

  it('renders footer content', async () => {
    await render(
      <TestContainer>
        <PrerenderedApp />
      </TestContainer>,
    )

    await expect
      .poll(
        () => {
          const el = page.getByText('the community').element()
          return el
        },
        { timeout: 5000 },
      )
      .not.toBeNull()
  })

  it('renders animated logo', async () => {
    await render(
      <TestContainer>
        <PrerenderedApp />
      </TestContainer>,
    )

    await expect
      .poll(
        () => {
          const logo = page.getByAltText('Spacewave Icon').element()
          return logo
        },
        { timeout: 5000 },
      )
      .not.toBeNull()
  })
})
