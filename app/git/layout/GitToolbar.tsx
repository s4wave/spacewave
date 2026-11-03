import {
  LuChevronLeft,
  LuChevronRight,
  LuChevronUp,
} from '@s4wave/web/images/icons.js'

import type { ListRefsResponse } from '@s4wave/sdk/git/repo.pb.js'

import { Toolbar } from '@s4wave/web/editors/file-browser/Toolbar.js'

import { RefSelector } from '../refs/RefSelector.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import { utilityState as themeUtilityState } from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    alignItems: 'center',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: themeColors['--color-panel-header'],
    paddingInline: themeLayout['--spacing'],
  },
  rule2: { display: 'flex' },
  rule3: {
    borderRadius: '.25rem',
    padding: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule4: {
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-pulldown-hover'],
      },
    },
  },
  rule5: { cursor: 'default', opacity: 0.4 },
  rule6: { minWidth: 0, flex: 1 },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
  },
})

// GitToolbarProps are props for the GitToolbar component.
export interface GitToolbarProps {
  effectiveRef: string | null
  refsResponse: ListRefsResponse | null
  refsLoading: boolean
  onRefSelect: (refName: string) => void
  currentPath: string
  onPathChange: (path: string) => void
  onBack?: () => void
  onForward?: () => void
  onUp?: () => void
  canGoBack: boolean
  canGoForward: boolean
  showPath?: boolean
}

// GitToolbar renders the toolbar with branch selector and path navigation.
export function GitToolbar({
  effectiveRef,
  refsResponse,
  refsLoading,
  onRefSelect,
  currentPath,
  onPathChange,
  onBack,
  onForward,
  onUp,
  canGoBack,
  canGoForward,
  showPath = true,
}: GitToolbarProps) {
  return (
    <div sx={stylexStyles2.rule1}>
      <RefSelector
        effectiveRef={effectiveRef}
        refsResponse={refsResponse}
        loading={refsLoading}
        onRefSelect={onRefSelect}
      />
      <div sx={stylexStyles2.rule2}>
        <button
          type="button"
          aria-label="Back"
          onClick={onBack}
          disabled={!canGoBack}
          sx={[
            stylexStyles2.rule3,
            canGoBack ? stylexStyles2.rule4 : stylexStyles2.rule5,
          ]}
        >
          <LuChevronLeft sx={stylexStyles.rule1} />
        </button>
        <button
          type="button"
          aria-label="Forward"
          onClick={onForward}
          disabled={!canGoForward}
          sx={[
            stylexStyles2.rule3,
            canGoForward ? stylexStyles2.rule4 : stylexStyles2.rule5,
          ]}
        >
          <LuChevronRight sx={stylexStyles.rule1} />
        </button>
        <button
          type="button"
          aria-label="Up one directory"
          onClick={onUp}
          disabled={currentPath === '/'}
          sx={[
            stylexStyles2.rule3,
            currentPath !== '/' ? stylexStyles2.rule4 : stylexStyles2.rule5,
          ]}
        >
          <LuChevronUp sx={stylexStyles.rule1} />
        </button>
      </div>
      {showPath && (
        <div sx={stylexStyles2.rule6}>
          <Toolbar
            currentPath={currentPath}
            onPathChange={onPathChange}
            onNavigate={onPathChange}
            height={28}
            hideNav
          />
        </div>
      )}
    </div>
  )
}
