import type { ReactNode } from 'react'
import '@fontsource-variable/manrope'
import { LuArrowLeft, LuRotateCw } from 'react-icons/lu'

import spacewaveIcon from '@s4wave/web/images/spacewave-icon.png'
import { cn } from '@s4wave/web/style/utils.js'

import type { LoadingView } from './types.js'
import { useReducedMotion } from './useReducedMotion.js'

interface LoadingScreenProps {
  view: LoadingView
  logo?: ReactNode
  topLeftSlot?: ReactNode
  // children present the owning operation’s current phases.
  children?: ReactNode
  footer?: ReactNode
  containerClassName?: string
}

// LoadingScreen keeps a stable, stylesheet-independent composition across
// startup boundaries. Callers retain readiness, progress, and recovery state.
export function LoadingScreen({
  view,
  logo,
  topLeftSlot,
  children,
  footer,
  containerClassName,
}: LoadingScreenProps) {
  const reducedMotion = useReducedMotion()
  const failed = view.state === 'error'
  const progress =
    view.progress === undefined || view.progressIndeterminate
      ? undefined
      : Math.max(0, Math.min(1, view.progress))

  return (
    <div
      className={cn('swl-canvas', containerClassName)}
      data-state={view.state}
      data-sw-reduced-motion={reducedMotion ? 'true' : undefined}
    >
      {topLeftSlot}
      <div className="swl-main">
        <div className="swl-emblem" aria-hidden="true">
          {logo ?? <img src={spacewaveIcon} alt="" width={56} height={56} />}
        </div>
        <div className="swl-console">
          <div className="swl-brand" aria-hidden="true">
            Spacewave
          </div>
          <div className="swl-head" aria-live="polite" aria-atomic="true">
            <h1 className="swl-title">{view.title}</h1>
          </div>
          {view.error ? (
            <p className="swl-error" role="alert">
              {view.error}
            </p>
          ) : null}
          {children ? (
            <div className="swl-phases">{children}</div>
          ) : view.detail ? (
            <p className="swl-detail">{view.detail}</p>
          ) : null}
          {!failed && progress !== undefined ? (
            <>
              <div className="swl-transfer">
                <span>Current download</span>
                <span>{Math.round(progress * 100)}%</span>
              </div>
              <div
                className="swl-transfer-track"
                role="progressbar"
                aria-label="Current download"
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={Math.round(progress * 100)}
              >
                <div
                  className="bg-brand progress-width h-full"
                  style={{ '--progress-width': `${progress * 100}%` }}
                />
              </div>
            </>
          ) : null}
          <div className="swl-footer">
            {view.onRetry ? (
              <button
                type="button"
                onClick={view.onRetry}
                className="swl-action swl-action--primary"
              >
                <LuRotateCw aria-hidden="true" />
                {view.retryLabel ?? 'Retry'}
              </button>
            ) : null}
            {view.onCancel ? (
              <button
                type="button"
                onClick={view.onCancel}
                className="swl-action"
              >
                <LuArrowLeft aria-hidden="true" />
                {view.cancelLabel ?? 'Back'}
              </button>
            ) : null}
            {footer}
          </div>
        </div>
      </div>
    </div>
  )
}
