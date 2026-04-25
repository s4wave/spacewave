import React from 'react'
import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'

import { StateDevToolsProvider } from './StateDevToolsContext.js'
import { StateTreeTab } from './StateTreeTab.js'
import {
  StateNamespaceProvider,
  atom,
  atomWithLocalStorage,
  useStateAtom,
  useStateNamespace,
} from '@s4wave/web/state/index.js'

function LocalRegisteredStateAtom() {
  const ns = useStateNamespace(['debug'])
  useStateAtom(ns, 'count', { value: 1 })
  return null
}

function PersistentRegisteredStateAtom() {
  const ns = useStateNamespace(['persisted'])
  useStateAtom(ns, 'flag', true)
  return null
}

describe('StateTreeTab', () => {
  it('renders grouped scope sections by default', () => {
    render(
      <StateDevToolsProvider>
        <StateTreeTab />
      </StateDevToolsProvider>,
    )

    expect(screen.getByText('Local State')).toBeDefined()
    expect(screen.getByText('Persistent State')).toBeDefined()
    expect(screen.getByText('Root State')).toBeDefined()
    expect(screen.getByText('Session State')).toBeDefined()
  })

  it('groups legacy atoms under local and persistent sections with scalar previews', () => {
    render(
      <StateDevToolsProvider>
        <StateNamespaceProvider rootAtom={atom({})}>
          <LocalRegisteredStateAtom />
          <StateNamespaceProvider
            rootAtom={atomWithLocalStorage('state-tree-tab-test', {})}
          >
            <PersistentRegisteredStateAtom />
          </StateNamespaceProvider>
          <StateTreeTab />
        </StateNamespaceProvider>
      </StateDevToolsProvider>,
    )

    expect(screen.getByText('count')).toBeDefined()
    expect(screen.getByText('{1}')).toBeDefined()
    expect(screen.getByText('flag')).toBeDefined()
    expect(screen.getByText('true')).toBeDefined()
  })
})
