import { LuExternalLink, LuTableProperties } from '@s4wave/web/images/icons.js'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { Button } from '@s4wave/web/ui/button.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import {
  SqlQueryResult,
  SqlQueryResultTypeID,
} from '@s4wave/sdk/sql/query-result/query-result.js'

import { flattenRowBatches } from './sql-cell.js'
import { SqlResultGrid } from './SqlResultGrid.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule4: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: { flex: 1 },
  rule6: { flex: 1, minHeight: 0 },
  rule7: { padding: `calc(${themeLayout['--spacing']} * 4)` },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

export { SqlQueryResultTypeID }

// SqlQueryResultViewer is the full sql/query-result viewer: a typed result grid
// over the persisted rows with NULL rendering, paging, and CSV export, plus
// links back to the source query and target database.
export function SqlQueryResultViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const container = SpaceContainerContext.useContextSafe()
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    SqlQueryResult,
    SqlQueryResultTypeID,
  )

  const gridResource = useResource(
    handle,
    async (result, signal) => {
      if (!result) return null
      return result.getResultGrid(signal)
    },
    [],
  )

  const grid = gridResource.value
  const data = grid
    ? {
        columns: grid.columns ?? [],
        rows: flattenRowBatches(grid.rowBatches),
        truncated: grid.truncated ?? false,
      }
    : null

  const navigate = (key: string | undefined) => {
    if (container && key) container.navigateToObjects([key])
  }

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <LuTableProperties sx={stylexStyles.rule1} aria-hidden />
        <span sx={stylexStyles2.rule3}>Query Result</span>
        {grid?.rowCount != null ? (
          <span sx={stylexStyles2.rule4}>{grid.rowCount.toString()} rows</span>
        ) : null}
        <div sx={stylexStyles2.rule5} />
        {container && grid?.sourceQueryObjectKey ? (
          <Button
            variant="ghost"
            size="xs"
            onClick={() => navigate(grid.sourceQueryObjectKey)}
          >
            <LuExternalLink sx={stylexStyles.rule2} />
            Query
          </Button>
        ) : null}
        {container && grid?.targetDbObjectKey ? (
          <Button
            variant="ghost"
            size="xs"
            onClick={() => navigate(grid.targetDbObjectKey)}
          >
            <LuExternalLink sx={stylexStyles.rule2} />
            Database
          </Button>
        ) : null}
      </div>

      <div sx={stylexStyles2.rule6}>
        {gridResource.loading && grid == null ? (
          <div sx={stylexStyles2.rule7}>
            <LoadingInline label="Loading result" tone="muted" />
          </div>
        ) : null}
        {gridResource.error ? (
          <div sx={stylexStyles2.rule7}>
            <ErrorState
              title="Query result unavailable"
              message={String(gridResource.error)}
              onRetry={gridResource.retry}
            />
          </div>
        ) : null}
        {grid?.error?.message ? (
          <div sx={stylexStyles2.rule7}>
            <ErrorState
              variant="inline"
              title="Query execution failed"
              message={grid.error.message}
            />
          </div>
        ) : null}
        {data && !grid?.error?.message ? (
          <SqlResultGrid
            data={data}
            csvFileName={`${objectKey.replaceAll('/', '-')}.csv`}
            emptyDescription="The query completed without returning rows."
          />
        ) : null}
      </div>
    </div>
  )
}
