import { useCallback, useEffect, useRef, useState } from 'react'
import { Document, Page, pdfjs } from 'react-pdf'
import { LuTriangleAlert } from '@s4wave/web/images/icons.js'

import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    minHeight: 0,
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    overflow: 'hidden',
  },
  rule2: {
    display: 'flex',
    minHeight: 0,
    width: '100%',
    maxWidth: 'var(--container-5xl)',
    flex: 1,
    flexDirection: 'column',
  },
  rule3: {
    display: 'flex',
    minHeight: 0,
    flex: 1,
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule4: {
    position: 'relative',
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 80)`,
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    overflow: 'hidden',
    padding: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
  },
  rule5: {
    position: 'absolute',
    bottom: `calc(${themeLayout['--spacing']} * 4)`,
    left: '50%',
    zIndex: 20,
    display: 'flex',
    '--tw-translate-x': 'calc(calc(1 / 2 * 100%) * -1)',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 78%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    color: themeColors['--color-foreground'],
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule6: {
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 80%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule7: {
    cursor: 'not-allowed',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    opacity: 0.5,
  },
  rule8: {
    minWidth: `calc(${themeLayout['--spacing']} * 20)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule9: {
    position: 'absolute',
    inset: 0,
    zIndex: 20,
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 82%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    textAlign: 'center',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule10: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    color: themeColors['--color-destructive'],
  },
  rule11: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule12: {
    maxWidth: 'var(--container-md)',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule13: {
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 60)`,
    alignItems: 'center',
    justifyContent: 'center',
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule14: { width: '100%', maxWidth: 'var(--container-sm)' },
  rule15: { maxHeight: '100%', maxWidth: '100%' },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

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
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <div
            ref={frameRef}
            aria-label={`PDF preview: ${title}`}
            sx={stylexStyles2.rule4}
          >
            {!!state.numPages && (
              <div sx={stylexStyles2.rule5}>
                <button
                  type="button"
                  sx={[stylexStyles2.rule6, !canGoPrev && stylexStyles2.rule7]}
                  disabled={!canGoPrev}
                  onClick={() => handleChangePage(-1)}
                >
                  Previous
                </button>
                <div sx={stylexStyles2.rule8}>
                  Page {state.pageNumber || 1} of {state.numPages}
                </div>
                <button
                  type="button"
                  sx={[stylexStyles2.rule6, !canGoNext && stylexStyles2.rule7]}
                  disabled={!canGoNext}
                  onClick={() => handleChangePage(1)}
                >
                  Next
                </button>
              </div>
            )}

            {state.error && (
              <div data-testid="unixfs-pdf-error" sx={stylexStyles2.rule9}>
                <div sx={stylexStyles2.rule10}>
                  <LuTriangleAlert sx={stylexStyles.rule1} />
                </div>
                <div sx={stylexStyles2.rule11}>PDF preview unavailable</div>
                <div sx={stylexStyles2.rule12}>{state.error}</div>
              </div>
            )}

            <Document
              file={inlineFileURL}
              loading={
                <div data-testid="unixfs-pdf-loading" sx={stylexStyles2.rule13}>
                  <div sx={stylexStyles2.rule14}>
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
                {...stylex.props(stylexStyles2.rule15)}
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
