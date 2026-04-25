import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup, fireEvent } from '@testing-library/react'
import { StateNamespaceProvider, atom } from '@s4wave/web/state/index.js'
import type { NotebookSource } from './proto/notebook.pb.js'
import NotebookSidebar from './NotebookSidebar.js'

function makeSources(...names: string[]): NotebookSource[] {
  return names.map((name) => ({ name, ref: `obj/-/${name.toLowerCase()}` }))
}

function renderSidebar(
  props: Partial<{
    sources: NotebookSource[]
    selectedSource: number
    onSelectSource: (index: number) => void
    onAddSource: () => void
    onRemoveSource: (index: number) => void
    onMoveSource: (index: number, delta: -1 | 1) => void
  }> = {},
) {
  const rootAtom = atom<Record<string, unknown>>({})
  return render(
    <StateNamespaceProvider rootAtom={rootAtom} namespace={['test']}>
      <NotebookSidebar
        sources={props.sources ?? []}
        selectedSource={props.selectedSource ?? 0}
        onSelectSource={props.onSelectSource ?? vi.fn()}
        onAddSource={props.onAddSource ?? vi.fn()}
        onRemoveSource={props.onRemoveSource ?? vi.fn()}
        onMoveSource={props.onMoveSource ?? vi.fn()}
        namespace={{ namespace: ['test'], stateAtom: rootAtom }}
      />
    </StateNamespaceProvider>,
  )
}

describe('NotebookSidebar', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders source names', () => {
    renderSidebar({ sources: makeSources('Docs', 'Notes', 'Archive') })
    expect(screen.getByText('Docs')).toBeDefined()
    expect(screen.getByText('Notes')).toBeDefined()
    expect(screen.getByText('Archive')).toBeDefined()
  })

  it('renders fallback name when source has no name', () => {
    renderSidebar({ sources: [{ ref: 'obj/-/path' }] })
    expect(screen.getByText('Source 1')).toBeDefined()
  })

  it('shows the Sources header', () => {
    renderSidebar({ sources: makeSources('A') })
    expect(screen.getByText('Sources')).toBeDefined()
  })

  it('highlights selected source with active selection class', () => {
    const sources = makeSources('Alpha', 'Beta')
    renderSidebar({ sources, selectedSource: 1 })
    const betaButton = screen.getByRole('button', { name: 'Beta' })
    expect(betaButton.parentElement?.className).toContain(
      'bg-list-active-selection-background',
    )
    const alphaButton = screen.getByRole('button', { name: 'Alpha' })
    expect(alphaButton.parentElement?.className).not.toContain(
      'bg-list-active-selection-background',
    )
  })

  it('calls onSelectSource when source is clicked', () => {
    const onSelectSource = vi.fn()
    renderSidebar({
      sources: makeSources('First', 'Second'),
      onSelectSource,
    })
    fireEvent.click(screen.getByText('Second'))
    expect(onSelectSource).toHaveBeenCalledWith(1)
  })

  it('shows empty state when no sources', () => {
    renderSidebar({ sources: [] })
    expect(screen.getByText('No sources configured')).toBeDefined()
  })

  it('toggles expand/collapse chevron on click', () => {
    renderSidebar({
      sources: makeSources('Expandable'),
    })
    const button = screen.getByRole('button', { name: 'Expandable' })
    fireEvent.click(button)
    fireEvent.click(button)
  })

  it('calls onAddSource when the add button is clicked', () => {
    const onAddSource = vi.fn()
    renderSidebar({
      sources: makeSources('Docs'),
      onAddSource,
    })
    fireEvent.click(screen.getByTitle('Add source'))
    expect(onAddSource).toHaveBeenCalledTimes(1)
  })

  it('calls onRemoveSource with the source index', () => {
    const onRemoveSource = vi.fn()
    renderSidebar({
      sources: makeSources('Docs', 'Archive'),
      onRemoveSource,
    })
    const removeButtons = screen.getAllByTitle('Remove source')
    fireEvent.click(removeButtons[1]!)
    expect(onRemoveSource).toHaveBeenCalledWith(1)
  })

  it('calls onMoveSource for the reorder controls', () => {
    const onMoveSource = vi.fn()
    renderSidebar({
      sources: makeSources('Docs', 'Archive'),
      onMoveSource,
    })
    const moveDownButtons = screen.getAllByTitle('Move source down')
    fireEvent.click(moveDownButtons[0]!)
    expect(onMoveSource).toHaveBeenCalledWith(0, 1)
  })
})
