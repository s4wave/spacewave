import { useCallback, useEffect, useRef, useState } from 'react'
import { Document, type DocumentProps, Page, pdfjs } from 'react-pdf'
import { LuChevronLeft, LuChevronRight, LuDownload } from 'react-icons/lu'

import {
  ViewerFloat,
  ViewerFloatLink,
  ViewerFloatPress,
  ViewerStage,
} from '@s4wave/web/object/ViewerFloat.js'
import { ViewerStatus } from '@s4wave/web/object/ViewerStatus.js'

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

// PdfFailure is why the document did not load and whether reading it again
// can succeed.
interface PdfFailure {
  message: string
  retryable: boolean
}

// PdfPreviewState is the frame's size, the document's load, and the page
// shown. attempt counts loads, so a Retry mounts a fresh Document.
interface PdfPreviewState {
  attempt: number
  containerHeight: number
  containerWidth: number
  error?: PdfFailure
  numPages?: number
  pageHeight?: number
  pageNumber: number
  pageWidth?: number
}

const initialPdfPreviewState: PdfPreviewState = {
  attempt: 0,
  containerHeight: 0,
  containerWidth: 0,
  pageNumber: 1,
}

// pdfFailure says what a PDF.js load error means for the person. A file PDF.js
// cannot parse or unlock fails the same way on every read, so it points at
// Download; any other failure, such as a stopped fetch, can succeed again.
function pdfFailure(error: Error): PdfFailure {
  if (error.name === 'InvalidPDFException') {
    return {
      message:
        'This file is not a valid PDF. Download it to open it elsewhere.',
      retryable: false,
    }
  }
  if (error.name === 'PasswordException') {
    return {
      message:
        'This PDF is protected by a password. Download it to open it elsewhere.',
      retryable: false,
    }
  }
  return {
    message: `Could not read the PDF: ${error.message || 'the read failed.'}`,
    retryable: true,
  }
}

// declinePassword answers pdf.js's password request with an Error, which
// pdf.js takes as a refusal: the load then fails with its PasswordException.
// react-pdf's default prompts with window.prompt, and its null answer on
// Cancel makes pdf.js ask again, forever. react-pdf types the callback as
// taking only a password, though pdf.js accepts an Error too.
const declinePassword: NonNullable<DocumentProps['onPassword']> = (
  callback,
) => {
  ;(callback as unknown as (refusal: Error) => void)(
    new Error('password required'),
  )
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
  // The frame the page fits, and the preview's state.
  const frameRef = useRef<HTMLDivElement | null>(null)
  const [state, setState] = useState<PdfPreviewState>(initialPdfPreviewState)

  // Follow the frame's size, so the page fits it.
  useEffect(() => {
    // The frame mounts with the surface.
    const frame = frameRef.current
    if (!frame) {
      return
    }

    // updateWidth keeps the frame's inner size, at least 240px each way.
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

    // Read the size now, then on each resize where the browser can watch it.
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

  // handleDocumentLoadSuccess shows the first page of the loaded document.
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

  // handleDocumentLoadError shows what the load failure means.
  const handleDocumentLoadError = useCallback((error: Error) => {
    setState((prev) => ({
      ...prev,
      error: pdfFailure(error),
    }))
  }, [])

  // handleRetry mounts a fresh Document, which reads the file again.
  const handleRetry = useCallback(() => {
    setState((prev) => ({
      ...prev,
      attempt: prev.attempt + 1,
      error: undefined,
      numPages: undefined,
      pageNumber: 1,
    }))
  }, [])

  // handleChangePage moves by offset pages, within the document.
  const handleChangePage = useCallback((offset: number) => {
    setState((prev) => ({
      ...prev,
      pageNumber: clampPage(prev.pageNumber + offset, prev.numPages),
    }))
  }, [])

  // handlePageLoadSuccess keeps the page's size, to fit it to the frame.
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

  // The page moves the person can make, and the width that fits the page.
  const canGoPrev = state.pageNumber > 1
  const canGoNext = !!state.numPages && state.pageNumber < state.numPages
  const fittedPageWidth = fitPageWidth({
    containerHeight: state.containerHeight,
    containerWidth: state.containerWidth,
    pageHeight: state.pageHeight,
    pageWidth: state.pageWidth,
  })

  return (
    <>
      {state.error ? (
        <ViewerStatus
          state="failed"
          message={state.error.message}
          retry={state.error.retryable ? handleRetry : undefined}
        />
      ) : (
        !state.numPages && (
          <ViewerStatus state="waiting" what="the PDF" rows={0} />
        )
      )}
      <ViewerStage media={state.error ? 'failed' : 'still'}>
        <div
          ref={frameRef}
          aria-label={`PDF preview: ${title}`}
          className="flex min-h-0 flex-1 items-center justify-center overflow-hidden p-3 sm:p-4"
        >
          {!state.error && (
            <Document
              key={state.attempt}
              file={inlineFileURL}
              suspense={false}
              loading={null}
              error={null}
              onLoadError={handleDocumentLoadError}
              onPassword={declinePassword}
              onSourceError={handleDocumentLoadError}
              onLoadSuccess={handleDocumentLoadSuccess}
            >
              <Page
                className="max-h-full max-w-full"
                loading={null}
                onLoadSuccess={handlePageLoadSuccess}
                pageNumber={state.pageNumber}
                renderAnnotationLayer={false}
                renderTextLayer={false}
                width={fittedPageWidth}
              />
            </Document>
          )}
        </div>

        <ViewerFloat at="tl" label="PDF">
          <ViewerFloatLink
            label="Download"
            icon={<LuDownload aria-hidden />}
            essential
            href={inlineFileURL}
            download={title}
          />
        </ViewerFloat>

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
      </ViewerStage>
    </>
  )
}

// UnixFSPdfFileViewer renders a dedicated inline preview surface for PDF
// files: one page on a stage, with its waiting or failed state in a row above.
export function UnixFSPdfFileViewer(props: UnixFSPdfFileViewerProps) {
  return <UnixFSPdfViewerSurface key={props.inlineFileURL} {...props} />
}
