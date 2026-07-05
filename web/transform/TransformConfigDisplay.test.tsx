import type React from 'react'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { Config as BlockEncConfig } from '@go/github.com/s4wave/spacewave/db/block/transform/blockenc/blockenc.pb.js'
import { BlockEnc } from '@go/github.com/s4wave/spacewave/db/util/blockenc/blockenc.pb.js'
import type { TransformInfo } from '@s4wave/sdk/space/space.pb.js'

import { TransformConfigDisplay } from './TransformConfigDisplay.js'

vi.mock('@s4wave/web/ui/tooltip.js', () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  TooltipContent: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
}))

describe('TransformConfigDisplay', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders AES-256-GCM for AES-256-GCM block encryption configs', () => {
    const info: TransformInfo = {
      steps: [
        {
          id: 'hydra/transform/blockenc',
          config: BlockEncConfig.toBinary({
            blockEnc: BlockEnc.BlockEnc_AES_256_GCM,
          }),
        },
      ],
    }

    render(<TransformConfigDisplay info={info} />)

    expect(screen.getByText('AES-256-GCM')).toBeDefined()
    expect(screen.queryByText('XChaCha20-Poly1305')).toBeNull()
  })
})
