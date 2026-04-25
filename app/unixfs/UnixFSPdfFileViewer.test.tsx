import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { UnixFSPdfFileViewer } from './UnixFSPdfFileViewer.js'

const h = vi.hoisted(() => ({
  documentFile: '',
  pageNumber: 0,
  pageWidth: 0,
  triggerError: false,
  workerSrc: '',
}))

vi.mock('react-pdf', async () => {
  const React = await import('react')
  return {
    pdfjs: {
      GlobalWorkerOptions: {
        get workerSrc() {
          return h.workerSrc
        },
        set workerSrc(value: string) {
          h.workerSrc = value
        },
      },
    },
    Document: ({
      children,
      file,
      onLoadError,
      onLoadSuccess,
    }: {
      children?: ReactNode
      file: string
      onLoadError?: (error: Error) => void
      onLoadSuccess?: (result: { numPages: number }) => void
    }) => {
      h.documentFile = file
      React.useEffect(() => {
        if (h.triggerError) {
          onLoadError?.(new Error('broken pdf'))
          return
        }
        onLoadSuccess?.({ numPages: 3 })
      }, [file, onLoadError, onLoadSuccess])
      return <div data-testid="react-pdf-document">{children}</div>
    },
    Page: ({
      pageNumber,
      width,
      onLoadSuccess,
    }: {
      pageNumber: number
      width?: number
      onLoadSuccess?: (page: {
        originalHeight: number
        originalWidth: number
      }) => void
    }) => {
      h.pageNumber = pageNumber
      h.pageWidth = width ?? 0
      React.useEffect(() => {
        onLoadSuccess?.({
          originalHeight: 1000,
          originalWidth: 800,
        })
      }, [onLoadSuccess])
      return (
        <div data-testid="react-pdf-page">
          Page {pageNumber} width {width ?? 0}
        </div>
      )
    },
  }
})

describe('UnixFSPdfFileViewer', () => {
  beforeEach(() => {
    h.documentFile = ''
    h.pageNumber = 0
    h.pageWidth = 0
    h.triggerError = false
    class ResizeObserverMock {
      observe(target: Element) {
        const width =
          target instanceof HTMLElement ? target.clientWidth || 720 : 720
        const height =
          target instanceof HTMLElement ? target.clientHeight || 540 : 540
        this.cb([{ contentRect: { height, width } }])
      }
      disconnect() {}
      constructor(
        private readonly cb: (
          entries: Array<{ contentRect: { height: number; width: number } }>,
        ) => void,
      ) {}
    }
    vi.stubGlobal('ResizeObserver', ResizeObserverMock)
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('passes the projected file url to Document and renders the first page', () => {
    render(
      <UnixFSPdfFileViewer
        title="guide.pdf"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/guide.pdf?inline=1"
      />,
    )

    expect(h.documentFile).toBe(
      '/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/guide.pdf?inline=1',
    )
    expect(screen.getByText('Page 1 of 3')).toBeDefined()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDefined()
    expect(screen.getByRole('button', { name: 'Next' })).toBeDefined()
    expect(h.pageNumber).toBe(1)
    expect(h.pageWidth).toBeGreaterThan(0)
    expect(h.workerSrc).toContain('pdf.worker.min.mjs')
  })

  it('navigates between pages after document load', () => {
    render(
      <UnixFSPdfFileViewer
        title="guide.pdf"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/guide.pdf?inline=1"
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(screen.getByText('Page 2 of 3')).toBeDefined()
    expect(h.pageNumber).toBe(2)

    fireEvent.click(screen.getByRole('button', { name: 'Previous' }))
    expect(screen.getByText('Page 1 of 3')).toBeDefined()
    expect(h.pageNumber).toBe(1)
  })

  it('renders an error state when the document fails to load', () => {
    h.triggerError = true

    render(
      <UnixFSPdfFileViewer title="broken.pdf" inlineFileURL="/broken.pdf" />,
    )

    expect(screen.getByTestId('unixfs-pdf-error')).toBeDefined()
    expect(screen.getByText('broken pdf')).toBeDefined()
  })
})
