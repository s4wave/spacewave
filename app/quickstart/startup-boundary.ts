import { markStartupBoundary, type StartupMarkDetail } from '@aptre/bldr'

export function markAppStartupBoundary(
  label: string,
  detail: StartupMarkDetail = {},
): void {
  markStartupBoundary(label, {
    source: 'app',
    ...detail,
  })
}

export function markQuickstartStartupBoundary(
  label: string,
  detail: StartupMarkDetail = {},
): void {
  markAppStartupBoundary(label, detail)
}
