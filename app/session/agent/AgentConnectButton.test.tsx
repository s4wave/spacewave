import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import { useBottomBarItems } from '@s4wave/web/frame/bottom-bar-context.js'
import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'

import { AgentConnectButton } from './AgentConnectButton.js'

const session = vi.hoisted(() => ({
  generatePairingCode: vi.fn(),
  watchPairingStatus: vi.fn(),
  watchResourcesList: vi.fn(),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: { useContext: () => ({ value: session }) },
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: (resource: { value: unknown }) => resource.value,
}))

vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: () => ({ providerId: 'local' }),
}))

vi.mock('@s4wave/web/hooks/useMountAccount.js', () => ({
  useMountAccount: () => ({ value: null }),
}))

vi.mock('../dashboard/SessionsSection.js', () => ({
  SessionsSection: () => null,
}))

vi.mock('@s4wave/app/hooks/useListenerStatus.js', () => ({
  useListenerStatus: () => null,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  usePath: () => '/u/1',
}))

// BarProbe renders the registered button the way the bottom bar does: from
// the item registry, not from the component that registered it.
function BarProbe() {
  const [selected, setSelected] = useState(false)
  const item = useBottomBarItems()[0]
  return <>{item?.button(selected, () => setSelected((s) => !s), '')}</>
}

describe('AgentConnectButton', () => {
  afterEach(() => {
    cleanup()
  })

  it('shows the pairing code once it arrives', async () => {
    session.generatePairingCode.mockResolvedValue('ABC123')
    session.watchPairingStatus.mockImplementation(async function* () {})
    vi.spyOn(navigator.clipboard, 'write').mockResolvedValue(undefined)

    render(
      <BottomBarRoot>
        <BarProbe />
        <AgentConnectButton />
      </BottomBarRoot>,
    )

    fireEvent.click(screen.getByTestId('agent-connect-button'))
    expect(await screen.findByText('ABC1 23')).toBeDefined()
    expect(
      await screen.findByText('Prompt copied. Paste it into your agent.'),
    ).toBeDefined()
  })
})
