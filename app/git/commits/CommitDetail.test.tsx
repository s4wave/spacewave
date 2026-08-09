import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'

import { CommitDetail } from './CommitDetail.js'

const mockPatchDiff = vi.hoisted(() =>
  vi.fn(({ patch }: { patch: string }) => (
    <pre data-testid="patch-diff">{patch}</pre>
  )),
)

vi.mock('@pierre/diffs/react', () => ({
  PatchDiff: mockPatchDiff,
}))

describe('CommitDetail', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('loads and renders the commit patch diff', async () => {
    const handle = {
      getCommit: vi.fn().mockResolvedValue({
        hash: 'abc1234567890',
        message: 'add file\n\nbody',
        authorName: 'Alice',
        authorEmail: 'alice@example.com',
        authorTimestamp: 1_700_000_000n,
        parentHashes: ['parent1234567890'],
      }),
      getDiffStat: vi.fn().mockResolvedValue({
        files: [{ path: 'README.md', additions: 2, deletions: 1 }],
      }),
      getDiffPatch: vi.fn().mockResolvedValue({
        patch: [
          'diff --git i/README.md w/README.md',
          '--- i/README.md',
          '+++ w/README.md',
          '@@ -1 +1 @@',
          '-old',
          '+new',
        ].join('\n'),
      }),
    }

    render(<CommitDetail handle={handle as never} commitHash="abc1234567890" />)

    await waitFor(() => {
      expect(handle.getDiffPatch).toHaveBeenCalledWith('abc1234567890')
    })

    expect(await screen.findByText('add file')).toBeTruthy()
    expect(screen.getByText('README.md')).toBeTruthy()
    expect(screen.getByTestId('patch-diff').textContent).toContain('+new')
    expect(mockPatchDiff).toHaveBeenCalledTimes(1)
  })

  it('shows when the commit patch is truncated', async () => {
    const handle = {
      getCommit: vi.fn().mockResolvedValue({
        hash: 'abc1234567890',
        message: 'large change',
        authorName: 'Alice',
        authorTimestamp: 1_700_000_000n,
      }),
      getDiffStat: vi.fn().mockResolvedValue({
        files: [
          { path: 'README.md', additions: 2, deletions: 1 },
          { path: 'large.bin', additions: 1000, deletions: 0 },
        ],
      }),
      getDiffPatch: vi.fn().mockResolvedValue({
        patch: [
          'diff --git i/README.md w/README.md',
          '--- i/README.md',
          '+++ w/README.md',
          '@@ -1 +1 @@',
          '-old',
          '+new',
        ].join('\n'),
        truncated: true,
        totalBytes: 1_048_576n,
        limitBytes: 524_288,
      }),
    }

    render(<CommitDetail handle={handle as never} commitHash="abc1234567890" />)

    await waitFor(() => {
      expect(handle.getDiffPatch).toHaveBeenCalledWith('abc1234567890')
    })

    expect(
      await screen.findByText(/Showing the first 512.0 KiB of 1.0 MiB/),
    ).toBeTruthy()
    expect(screen.getByText('README.md')).toBeTruthy()
    expect(screen.queryByText('large.bin')).toBeNull()
  })
})
