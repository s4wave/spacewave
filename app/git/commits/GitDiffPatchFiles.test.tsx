import { execFileSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { GitDiffPatchFiles } from './GitDiffPatchFiles.js'

const mockPatchDiff = vi.hoisted(() =>
  vi.fn(({ patch }: { patch: string }) => (
    <pre data-testid="patch-diff">{patch}</pre>
  )),
)

vi.mock('@pierre/diffs/react', () => ({ PatchDiff: mockPatchDiff }))

const fixtureDirs: string[] = []

function git(cwd: string, ...args: string[]): string {
  return execFileSync('git', args, { cwd, encoding: 'utf8' })
}

function write(cwd: string, path: string, contents: string) {
  mkdirSync(dirname(join(cwd, path)), { recursive: true })
  writeFileSync(join(cwd, path), contents)
}

interface NativeFixture {
  patch: string
  files: Array<{ path: string; additions: number; deletions: number }>
}

function nativeFixture(
  change: (cwd: string) => NativeFixture['files'],
): NativeFixture {
  const cwd = mkdtempSync(join(tmpdir(), 'spacewave-git-patch-'))
  fixtureDirs.push(cwd)
  git(cwd, 'init', '-q')
  git(cwd, 'config', 'user.name', 'Spacewave Test')
  git(cwd, 'config', 'user.email', 'spacewave@example.invalid')
  git(cwd, 'config', 'core.quotePath', 'true')
  write(cwd, 'rename source.txt', 'rename\n')
  write(cwd, 'copy source.txt', 'copy\n')
  write(cwd, 'delete me.txt', 'delete\n')
  write(cwd, 'space only.txt', 'before\n')
  write(cwd, 'source', 'source before\n')
  write(cwd, 'victim w/source', 'victim before\n')
  git(cwd, 'add', '.')
  git(cwd, 'commit', '-qm', 'base')
  const files = change(cwd)
  git(cwd, 'add', '-A')
  return {
    files,
    patch: git(
      cwd,
      'diff',
      '--cached',
      '-M',
      '-C',
      '--find-copies-harder',
      '--src-prefix=i/',
      '--dst-prefix=w/',
    ),
  }
}

function expectJoined(fixture: NativeFixture) {
  render(
    <GitDiffPatchFiles
      files={fixture.files}
      patch={fixture.patch}
      loading={false}
    />,
  )
  expect(screen.getAllByTestId('patch-diff')).toHaveLength(fixture.files.length)
  for (const file of fixture.files) {
    expect(
      screen.getByText(
        (_content, element) => element?.textContent === file.path,
        {
          selector: 'span',
        },
      ),
    ).toBeTruthy()
  }
}

describe('GitDiffPatchFiles', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    for (const dir of fixtureDirs.splice(0)) rmSync(dir, { recursive: true })
  })

  it('joins the real ambiguous source and victim w/source headers uniquely', () => {
    const fixture = nativeFixture((cwd) => {
      write(cwd, 'source', 'source after\n')
      write(cwd, 'victim w/source', 'victim after\n')
      return [
        { path: 'source', additions: 1, deletions: 1 },
        { path: 'victim w/source', additions: 1, deletions: 1 },
      ]
    })
    expect(fixture.patch).toContain('diff --git i/source w/source')
    expect(fixture.patch).toContain(
      'diff --git i/victim w/source w/victim w/source',
    )

    expectJoined(fixture)
    const patches = mockPatchDiff.mock.calls.map(([props]) => props.patch)
    expect(patches).toHaveLength(2)
    expect(patches.find((patch) => patch.includes('+source after'))).toContain(
      'diff --git i/source w/source',
    )
    expect(patches.find((patch) => patch.includes('+victim after'))).toContain(
      'diff --git i/victim w/source w/victim w/source',
    )
  })

  it('joins an unquoted native path containing spaces', () => {
    const fixture = nativeFixture((cwd) => {
      write(cwd, 'space only.txt', 'after\n')
      return [{ path: 'space only.txt', additions: 1, deletions: 1 }]
    })
    expect(fixture.patch).toContain(
      'diff --git i/space only.txt w/space only.txt',
    )
    expectJoined(fixture)
  })

  it('joins native rename and copy headers', () => {
    const fixture = nativeFixture((cwd) => {
      git(cwd, 'mv', 'rename source.txt', 'renamed path.txt')
      write(cwd, 'copied path.txt', 'copy\n')
      return [
        { path: 'copied path.txt', additions: 0, deletions: 0 },
        { path: 'renamed path.txt', additions: 0, deletions: 0 },
      ]
    })
    expect(fixture.patch).toContain('copy from copy source.txt')
    expect(fixture.patch).toContain('rename from rename source.txt')
    expectJoined(fixture)
  })

  it('joins native additions and deletions', () => {
    const fixture = nativeFixture((cwd) => {
      rmSync(join(cwd, 'delete me.txt'))
      write(cwd, 'added path.txt', 'added\n')
      return [
        { path: 'added path.txt', additions: 1, deletions: 0 },
        { path: 'delete me.txt', additions: 0, deletions: 1 },
      ]
    })
    expect(fixture.patch).toContain('--- /dev/null')
    expect(fixture.patch).toContain('+++ /dev/null')
    expectJoined(fixture)
  })

  it('decodes native C-quoted paths with standard and octal UTF-8 escapes', () => {
    const path = 'quoted\t"é.txt'
    const fixture = nativeFixture((cwd) => {
      write(cwd, path, 'quoted\n')
      return [{ path, additions: 1, deletions: 0 }]
    })
    expect(fixture.patch).toContain('\\t\\"\\303\\251.txt"')
    expectJoined(fixture)
  })

  it.each([
    ['unterminated quote', 'diff --git "i/a" "w/a'],
    ['unknown escape', 'diff --git "i/a\\q" "w/a"'],
    ['short octal escape', 'diff --git "i/a\\12" "w/a"'],
    ['invalid UTF-8', 'diff --git "i/a\\303" "w/a"'],
    ['extra quoted token', 'diff --git "i/a" "w/a" "w/evil"'],
    ['wrong prefixes', 'diff --git a/a b/a'],
  ])('rejects %s in a native header', (_label, header) => {
    render(
      <GitDiffPatchFiles
        files={[{ path: 'a', additions: 1, deletions: 0 }]}
        patch={`${header}\n@@ -0,0 +1 @@\n+bad`}
        loading={false}
      />,
    )
    expect(screen.queryByTestId('patch-diff')).toBeNull()
  })

  it('rejects malicious duplicate claims for one authoritative path', () => {
    render(
      <GitDiffPatchFiles
        files={[{ path: 'a', additions: 1, deletions: 0 }]}
        patch={[
          'diff --git i/a w/a\n--- i/a\n+++ w/a\n@@ -0,0 +1 @@\n+first',
          'diff --git i/a w/a\n--- i/a\n+++ w/a\n@@ -0,0 +1 @@\n+second',
        ].join('\n')}
        loading={false}
      />,
    )
    expect(screen.queryByTestId('patch-diff')).toBeNull()
  })

  it('keeps DiffFileStat paths when a truncated patch ends between files', () => {
    const fixture = nativeFixture((cwd) => {
      write(cwd, 'space only.txt', 'after\n')
      write(cwd, 'added path.txt', 'added\n')
      return [
        { path: 'added path.txt', additions: 1, deletions: 0 },
        { path: 'space only.txt', additions: 1, deletions: 1 },
      ]
    })
    const firstChunk = fixture.patch.split('\ndiff --git ', 1)[0]
    render(
      <GitDiffPatchFiles
        files={fixture.files}
        patch={firstChunk}
        loading={false}
        truncated
        totalBytes={1000}
        limitBytes={500}
      />,
    )
    expect(screen.getByText(/Showing the first/)).toBeTruthy()
    expect(screen.getAllByTestId('patch-diff')).toHaveLength(1)
    expect(screen.getByText(fixture.files[0]!.path)).toBeTruthy()
  })
})
