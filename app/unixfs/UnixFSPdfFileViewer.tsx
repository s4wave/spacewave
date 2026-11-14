import { useCallback, useEffect, useRef, useState } from 'react'
import { Document, Page, pdfjs } from 'react-pdf'
import { LuChevronLeft, LuChevronRight, LuTriangleAlert } from 'react-icons/lu'

import {
  ViewerFloat,
  ViewerFloatPress,
  ViewerStage,
} from '@s4wave/web/object/ViewerFloat.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

pdfjs.GlobalWorkerOptions.workerSrc = new URL(
  'pdfjs-dist/build/pdf.worker.min.mjs',
  import.meta.url,
).toString()

// UnixFSPdfFileViewerProps are the props passed to the UnixFSPdfFileViewer.
export interface UnixFSPdfFileViewerProps {
  // title is the accessible label for the PDF preview.
  title: string
  // inlineFileURL is the projected raw file URL used for the PDF document.
  inlineFileURL: string
}

interface PdfPreviewState {
  containerHeight: number
  containerWidth: number
  error?: string
  numPages?: number
  pageHeight?: number
  pageNumber: number
  pageWidth?: number
}

const initialPdfPreviewState: PdfPreviewState = {
  containerHeight: 0,
  containerWidth: 0,
  pageNumber: 1,
}

function buildPdfErrorMessage(error: Error): string {
  return error.message || 'This PDF file could not be loaded.'
}

function clampPage(pageNumber: number, numPages?: number): number {
  if (!numPages) {
    return 1
  }
  if (pageNumber < 1) {
    return 1
  }
  if (pageNumber > numPages) {
    return numPages
  }
  return pageNumber
}

function fitPageWidth({
  containerHeight,
  containerWidth,
  pageHeight,
  pageWidth,
}: {
  containerHeight: number
  containerWidth: number
  pageHeight?: number
  pageWidth?: number
}): number | undefined {
  if (!containerWidth) {
    return undefined
  }

  if (!pageWidth || !pageHeight || !containerHeight) {
    return containerWidth
  }

  const widthByHeight = Math.floor((containerHeight * pageWidth) / pageHeight)
  return Math.max(Math.min(containerWidth, widthByHeight), 1)
}

function UnixFSPdfViewerSurface({
  title,
  inlineFileURL,
}: UnixFSPdfFileViewerProps) {
  const frameRef = useRef<HTMLDivElement | null>(null)
  const [state, setState] = useState<PdfPreviewState>(initialPdfPreviewState)

  useEffect(() => {
    const frame = frameRef.current
    if (!frame) {
      return
    }

    const updateWidth = (width: number) => {
      const nextWidth = Math.max(Math.floor(width) - 32, 240)
      const nextHeight = Math.max(Math.floor(frame.clientHeight) - 32, 240)
      setState((prev) => {
        if (
          prev.containerWidth === nextWidth &&
          prev.containerHeight === nextHeight
        ) {
          return prev
        }
        return {
          ...prev,
          containerHeight: nextHeight,
          containerWidth: nextWidth,
        }
      })
    }

    updateWidth(frame.clientWidth)

    if (typeof ResizeObserver === 'undefined') {
      return
    }

    const observer = new ResizeObserver((entries) => {
      const entry = entries[0]
      if (!entry) {
        return
      }
      updateWidth(entry.contentRect.width)
    })
    observer.observe(frame)
    return () => {
      observer.disconnect()
    }
  }, [])

  const handleDocumentLoadSuccess = useCallback(
    ({ numPages }: { numPages: number }) => {
      setState((prev) => ({
        ...prev,
        error: undefined,
        numPages,
        pageNumber: clampPage(1, numPages),
      }))
    },
    [],
  )

  const handleDocumentLoadError = useCallback((error: Error) => {
    setState((prev) => ({
      ...prev,
      error: buildPdfErrorMessage(error),
    }))
  }, [])

  const handleChangePage = useCallback((offset: number) => {
    setState((prev) => ({
      ...prev,
      pageNumber: clampPage(prev.pageNumber + offset, prev.numPages),
    }))
  }, [])

  const handlePageLoadSuccess = useCallback(
    (page: { originalHeight: number; originalWidth: number }) => {
      setState((prev) => {
        if (
          prev.pageHeight === page.originalHeight &&
          prev.pageWidth === page.originalWidth
        ) {
          return prev
        }
        return {
          ...prev,
          pageHeight: page.originalHeight,
          pageWidth: page.originalWidth,
        }
      })
    },
    [],
  )

  const canGoPrev = state.pageNumber > 1
  const canGoNext = !!state.numPages && state.pageNumber < state.numPages
  const fittedPageWidth = fitPageWidth({
    containerHeight: state.containerHeight,
    containerWidth: state.containerWidth,
    pageHeight: state.pageHeight,
    pageWidth: state.pageWidth,
  })

  return (
    <ViewerStage media={state.error ? 'failed' : 'still'}>
      <div
        ref={frameRef}
        aria-label={`PDF preview: ${title}`}
        className="flex min-h-0 flex-1 items-center justify-center overflow-hidden p-3 sm:p-4"
      >
        <Document
          file={inlineFileURL}
          loading={
            <div
              data-testid="unixfs-pdf-loading"
              className="flex min-h-60 items-center justify-center p-6"
            >
              <div className="w-full max-w-sm">
                <LoadingCard
                  view={{
                    state: 'active',
                    title: 'Loading preview',
                    detail: 'Waiting for PDF metadata.',
                  }}
                />
              </div>
            </div>
          }
          onLoadError={handleDocumentLoadError}
          onLoadSuccess={handleDocumentLoadSuccess}
        >
          <Page
            className="max-h-full max-w-full"
            onLoadSuccess={handlePageLoadSuccess}
            pageNumber={state.pageNumber}
            renderAnnotationLayer={false}
            renderTextLayer={false}
            width={fittedPageWidth}
          />
        </Document>
      </div>

      {!!state.numPages && (
        <ViewerFloat at="bc" label="Pages">
          <ViewerFloatPress
            label="Previous page"
            icon={<LuChevronLeft aria-hidden />}
            essential
            disabled={!canGoPrev}
            onClick={() => handleChangePage(-1)}
          />
          <span className="text-foreground-alt px-1 text-xs tabular-nums">
            Page {state.pageNumber || 1} of {state.numPages}
          </span>
          <ViewerFloatPress
            label="Next page"
            icon={<LuChevronRight aria-hidden />}
            essential
            disabled={!canGoNext}
            onClick={() => handleChangePage(1)}
          />
        </ViewerFloat>
      )}

      {state.error && (
        <div
          data-testid="unixfs-pdf-error"
          className="bg-background/82 absolute inset-0 z-20 flex flex-col items-center justify-center gap-3 p-6 text-center backdrop-blur-sm"
        >
          <div className="bg-destructive/10 text-destructive flex size-10 items-center justify-center rounded-full">
            <LuTriangleAlert className="size-5" />
          </div>
          <div className="text-foreground text-sm font-semibold">
            PDF preview unavailable
          </div>
          <div className="text-foreground-alt max-w-md text-xs">
            {state.error}
          </div>
        </div>
      )}
    </ViewerStage>
  )
}

// UnixFSPdfFileViewer renders a dedicated inline preview surface for PDF files.
export function UnixFSPdfFileViewer(props: UnixFSPdfFileViewerProps) {
  return <UnixFSPdfViewerSurface key={props.inlineFileURL} {...props} />
}
