import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { StaticProvider } from '@s4wave/app/prerender/StaticContext.js'
import offer from '@s4wave/core/provider/spacewave/api/cloud-offer.json'
import { SessionIndexContext } from '@s4wave/web/contexts/SessionIndexContext.js'
import { RouterProvider } from '@s4wave/web/router/router.js'

import policies from './customer-policies.json'
import { PrivacyPolicy } from './PrivacyPolicy.js'
import { TermsOfService } from './TermsOfService.js'

describe('canonical customer policies', () => {
  afterEach(cleanup)

  it('renders the accepted offer and processing addendum with public links', () => {
    render(
      <RouterProvider path="/tos" onNavigate={() => {}}>
        <StaticProvider>
          <TermsOfService />
        </StaticProvider>
      </RouterProvider>,
    )
    expect(
      screen.getByText(`Policy version: ${offer.policyVersion}`),
    ).toBeTruthy()
    expect(screen.getByText(/includes 100 GiB/).textContent).toContain(
      `${offer.storageBytes.toLocaleString('en-US')} bytes`,
    )
    expect(screen.getByText(/Optional storage above/).textContent).toContain(
      `$${offer.storageMicrodollarsPerGibMonth / 1_000_000} per GiB-month`,
    )
    expect(screen.getByText(/Optional storage above/).textContent).toContain(
      `Checkout initially selects $${offer.defaultOverageLimitCents / 100}`,
    )
    expect(
      screen.getByRole('heading', {
        name: 'Business Data Processing Addendum',
      }),
    ).toBeTruthy()
    expect(
      screen
        .getAllByRole('link', { name: 'Privacy Policy' })[0]
        .getAttribute('href'),
    ).toBe('/privacy')
    expect(screen.getByText('DRAFT')).toBeTruthy()
    expect(policies.terms.version).toBe(policies.privacy.version)
  })

  it('keeps privacy and copyright navigation inside the signed-in session', () => {
    render(
      <RouterProvider path="/u/7/legal/privacy" onNavigate={() => {}}>
        <SessionIndexContext value={7}>
          <PrivacyPolicy />
        </SessionIndexContext>
      </RouterProvider>,
    )
    expect(
      screen
        .getAllByRole('link', { name: 'Terms of Service' })[0]
        .getAttribute('href'),
    ).toBe('#/u/7/legal/tos')
    expect(
      screen.getByRole('link', { name: 'DMCA Policy' }).getAttribute('href'),
    ).toBe('#/u/7/legal/dmca')
    expect(
      screen.getByText(/ninety days after provider acceptance/),
    ).toBeTruthy()
    expect(screen.getByText(/twenty-four-hour read-only hold/)).toBeTruthy()
  })
})
