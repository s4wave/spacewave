import { afterEach, describe, expect, it, vi } from 'vitest'
import { page, userEvent } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

import '@s4wave/web/style/app.css'
import { StateNamespaceProvider, atom } from '@s4wave/web/state/index.js'

const fixture = vi.hoisted(() => {
  const content =
    '# Welcome to your notebook\n\nA note you can read on a phone.'
  const bytes = new TextEncoder().encode(content)
  const file = {
    getSize: vi.fn(async () => BigInt(bytes.length)),
    readAt: vi.fn(async () => ({ data: bytes, eof: true })),
    writeAt: vi.fn(async () => {}),
    truncate: vi.fn(async () => {}),
    release: vi.fn(),
  }
  return {
    content,
    file,
    directory: { lookup: vi.fn(async () => file) },
  }
})

const ready = vi.hoisted(() => <T,>(value: T) => ({
  value,
  loading: false,
  error: null,
  retry: vi.fn(),
}))

vi.mock('@s4wave/web/object/object.js', () => ({
  getObjectKey: () => 'fixture-notebook',
}))

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => {
  const resource = ready(null)
  return { useAccessTypedHandle: () => resource }
})

vi.mock('./useWorldObjectMessageState.js', () => {
  const result = {
    state: ready({
      sources: [{ name: 'Phone notes', ref: 'fixture-files/-/notes' }],
    }),
    sources: [{ name: 'Phone notes', ref: 'fixture-files/-/notes' }],
  }
  return { useWorldObjectMessageState: () => result }
})

vi.mock('./useNotebookSavedViews.js', () => ({
  useNotebookSavedViews: () => ({
    views: [],
    loading: false,
    pending: false,
    error: null,
    ready: false,
    save: vi.fn(),
    rename: vi.fn(),
    remove: vi.fn(),
    retry: vi.fn(),
  }),
}))

vi.mock('@s4wave/sdk/space/object-uri.js', () => ({
  parseObjectUri: () => ({ objectKey: 'fixture-files', path: 'notes' }),
}))

vi.mock('@s4wave/web/hooks/useUnixFSHandle.js', () => {
  const directory = ready(fixture.directory)
  const file = ready(fixture.file)
  const entries = ready([{ name: 'welcome.md', isDir: false }])
  const content = ready(fixture.content)
  return {
    useUnixFSRootHandle: () => directory,
    useUnixFSHandle: (_root: unknown, path: string) =>
      path.endsWith('.md') ? file : directory,
    useUnixFSHandleEntries: () => entries,
    useUnixFSHandleTextContent: () => content,
  }
})

import NotebookViewer from './NotebookViewer.js'

const worldState = ready(null)
const rootAtom = atom<Record<string, unknown>>({})

/** NotebookFixture mounts the production viewer contract with disposable note resources. */
function NotebookFixture() {
  return (
    <div className="h-dvh w-full">
      <StateNamespaceProvider rootAtom={rootAtom} namespace={['phone-fixture']}>
        <NotebookViewer
          objectInfo={
            { key: 'fixture-notebook', typeId: 'notes/notebook' } as never
          }
          worldState={worldState as never}
        />
      </StateNamespaceProvider>
    </div>
  )
}

describe('NotebookViewer phone layout', () => {
  afterEach(() => {
    void cleanup()
  })

  const coarse = matchMedia('(pointer: coarse)').matches
  const viewports = [
    [390, 844],
    [360, 780],
  ]
  if (coarse) viewports.push([844, 390])

  for (const [width, height] of viewports) {
    it(`keeps notebook actions in view at ${width}x${height}`, async () => {
      await page.viewport(width, height)
      await render(<NotebookFixture />)
      const capture = (state: string) =>
        page.screenshot({
          path: `__screenshots__/notes-phone/${width}x${height}-${import.meta.env.VITE_NOTES_ENGINE || 'chromium'}-${state}-after.png`,
        })

      const note = page.getByTestId('notes-note-row')
      await expect.element(note).toBeVisible()
      const noteRect = (note.element() as HTMLElement).getBoundingClientRect()
      expect(noteRect.width).toBeGreaterThan(100)
      expect(noteRect.height).toBeGreaterThanOrEqual(44)
      expect(noteRect.right).toBeLessThanOrEqual(width)
      const searchRect = page
        .getByRole('textbox', { name: 'Search notes' })
        .element()
        .getBoundingClientRect()
      expect(searchRect.height).toBeGreaterThanOrEqual(44)
      expect(searchRect.width).toBeGreaterThanOrEqual(44)
      const sortRect = page
        .getByRole('combobox', { name: 'Sort notes' })
        .element()
        .getBoundingClientRect()
      expect(sortRect.height).toBeGreaterThanOrEqual(44)
      expect(sortRect.width).toBeGreaterThanOrEqual(44)
      for (const name of [
        'New folder',
        'New note',
        'New Org note',
        'Rename note',
        'Delete note',
      ]) {
        const rect = page
          .getByTitle(name, { exact: true })
          .element()
          .getBoundingClientRect()
        expect(rect.width).toBeGreaterThanOrEqual(44)
        expect(rect.height).toBeGreaterThanOrEqual(44)
      }
      await capture('list')

      await userEvent.click(
        page.getByRole('button', { name: 'Open notebook navigation' }),
      )
      const addSource = page.getByRole('button', { name: 'Add source' })
      await expect.element(addSource).toBeVisible()
      const addSourceRect = (
        addSource.element() as HTMLElement
      ).getBoundingClientRect()
      expect(addSourceRect.height).toBeGreaterThanOrEqual(44)
      expect(addSourceRect.right).toBeLessThanOrEqual(width)
      await userEvent.click(page.getByText('Shared views', { exact: true }))
      const savedViewRect = page
        .getByRole('combobox', { name: 'Saved view' })
        .element()
        .getBoundingClientRect()
      expect(savedViewRect.height).toBeGreaterThanOrEqual(44)
      const deleteRect = page
        .getByRole('button', { name: 'Delete', exact: true })
        .element()
        .getBoundingClientRect()
      expect(deleteRect.width).toBeGreaterThanOrEqual(44)
      expect(deleteRect.height).toBeGreaterThanOrEqual(44)
      await capture('navigation')
      await userEvent.click(page.getByText('Shared views', { exact: true }))
      await userEvent.click(
        page.getByRole('button', { name: 'Close notebook navigation' }),
      )

      await userEvent.click(note)
      const content = page.getByTestId('notes-content-view')
      await expect.element(content).toBeVisible()
      const contentRect = (
        content.element() as HTMLElement
      ).getBoundingClientRect()
      expect(contentRect.width).toBeGreaterThan(300)
      expect(contentRect.right).toBeLessThanOrEqual(width)
      expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(width)

      const toggle = page.getByTestId('notes-source-toggle')
      const toggleRect = (
        toggle.element() as HTMLElement
      ).getBoundingClientRect()
      expect(toggleRect.height).toBeGreaterThanOrEqual(44)
      const boldRect = (
        page.getByTitle('Bold (Ctrl+B)').element() as HTMLElement
      ).getBoundingClientRect()
      expect(boldRect.width).toBeGreaterThanOrEqual(44)
      expect(boldRect.height).toBeGreaterThanOrEqual(44)
      await capture('reading')
      await userEvent.click(toggle)
      await expect
        .element(page.getByRole('textbox', { name: 'Note source' }))
        .toBeVisible()

      await capture('source')

      await userEvent.click(page.getByRole('button', { name: 'Back to notes' }))
      await expect.element(note).toBeVisible()
    })
  }

  it.skipIf(coarse)('keeps the three compact panels on desktop', async () => {
    await page.viewport(1024, 768)
    await render(<NotebookFixture />)
    await expect.element(page.getByTestId('notes-note-row')).toBeVisible()

    const sidebar = document.querySelector('details')?.parentElement
    const list = document.querySelector('[data-testid="notes-note-list"]')
      ?.parentElement?.parentElement
    expect(sidebar?.getBoundingClientRect().width).toBe(200)
    expect(list?.getBoundingClientRect().width).toBe(250)
    expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(1024)
  })
})
