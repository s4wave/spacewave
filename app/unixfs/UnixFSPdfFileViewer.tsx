import { useCallback, useEffect, useRef, useState } from 'react'
import { Document, Page, pdfjs } from 'react-pdf'
import { LuTriangleAlert } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
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
    <div className="flex min-h-0 flex-1 flex-col items-center justify-center overflow-hidden">
      <div className="flex min-h-0 w-full max-w-5xl flex-1 flex-col">
        <div className="border-foreground/6 bg-background-card/30 flex min-h-0 flex-1 overflow-hidden rounded-lg border">
          <div
            ref={frameRef}
            aria-label={`PDF preview: ${title}`}
            className="relative flex min-h-[320px] flex-1 items-center justify-center overflow-hidden p-3 sm:p-4"
          >
            {!!state.numPages && (
              <div className="border-foreground/10 bg-background/78 text-foreground absolute bottom-4 left-1/2 z-20 flex -translate-x-1/2 items-center gap-2 rounded-full border px-2 py-1.5 shadow-sm backdrop-blur-sm select-none">
                <button
                  type="button"
                  className={cn(
                    'border-foreground/8 bg-background-card/60 hover:bg-background-card/80 hover:border-foreground/15 rounded-full border px-3 py-1 text-xs font-medium transition-all duration-150 select-none',
                    !canGoPrev &&
                      'text-foreground-alt/40 cursor-not-allowed opacity-50',
                  )}
                  disabled={!canGoPrev}
                  onClick={() => handleChangePage(-1)}
                >
                  Previous
                </button>
                <div className="text-foreground-alt min-w-20 text-center text-xs font-medium select-none">
                  Page {state.pageNumber || 1} of {state.numPages}
                </div>
                <button
                  type="button"
                  className={cn(
                    'border-foreground/8 bg-background-card/60 hover:bg-background-card/80 hover:border-foreground/15 rounded-full border px-3 py-1 text-xs font-medium transition-all duration-150 select-none',
                    !canGoNext &&
                      'text-foreground-alt/40 cursor-not-allowed opacity-50',
                  )}
                  disabled={!canGoNext}
                  onClick={() => handleChangePage(1)}
                >
                  Next
                </button>
              </div>
            )}

            {state.error && (
              <div
                data-testid="unixfs-pdf-error"
                className="bg-background/82 absolute inset-0 z-20 flex flex-col items-center justify-center gap-3 p-6 text-center backdrop-blur-sm"
              >
                <div className="bg-destructive/10 text-destructive flex h-10 w-10 items-center justify-center rounded-full">
                  <LuTriangleAlert className="h-5 w-5" />
                </div>
                <div className="text-foreground text-sm font-semibold">
                  PDF preview unavailable
                </div>
                <div className="text-foreground-alt max-w-md text-xs">
                  {state.error}
                </div>
              </div>
            )}

            <Document
              file={inlineFileURL}
              loading={
                <div
                  data-testid="unixfs-pdf-loading"
                  className="flex min-h-[240px] items-center justify-center p-6"
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
        </div>
      </div>
    </div>
  )
}

// UnixFSPdfFileViewer renders a dedicated inline preview surface for PDF files.
export function UnixFSPdfFileViewer(props: UnixFSPdfFileViewerProps) {
  return <UnixFSPdfViewerSurface key={props.inlineFileURL} {...props} />
}
