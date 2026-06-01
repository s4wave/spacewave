import { useState } from 'react'
import { createRoot } from 'react-dom/client'

import '@s4wave/web/style/app.css'

import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'
import { ViewerFrame } from '@s4wave/web/frame/ViewerFrame.js'

interface ProofState {
  count: number
  events: string[]
}

declare global {
  interface Window {
    __bottomBarContextMenuProof?: ProofState
  }
}

document.documentElement.classList.add('dark')
window.__bottomBarContextMenuProof = { count: 0, events: [] }

function button(label: string) {
  return (selected: boolean, onClick: () => void, className?: string) => (
    <BottomBarItem selected={selected} onClick={onClick} className={className}>
      {label}
    </BottomBarItem>
  )
}

function ProofHarness() {
  const [openMenu, setOpenMenu] = useState('')
  const [count, setCount] = useState(0)

  return (
    <div className="h-dvh w-dvw">
      <BottomBarRoot openMenu={openMenu} setOpenMenu={setOpenMenu}>
        <BottomBarLevel
          id="space"
          menuLabel="Space"
          button={button('Space')}
          contextMenuItems={[
            {
              type: 'action',
              id: 'switch-object-here',
              label: 'Switch Object Here',
              onSelect: ({ openKind }) => {
                const proof = window.__bottomBarContextMenuProof
                if (!proof) return
                const nextCount = proof.count + 1
                proof.count = nextCount
                proof.events.push(openKind)
                setCount(nextCount)
              },
            },
          ]}
        >
          <ViewerFrame>
            <div data-testid="content" className="p-4">
              Browser proof content
            </div>
          </ViewerFrame>
        </BottomBarLevel>
      </BottomBarRoot>
      <div id="proof-count" data-count={count} />
    </div>
  )
}

createRoot(document.getElementById('root')!).render(<ProofHarness />)
