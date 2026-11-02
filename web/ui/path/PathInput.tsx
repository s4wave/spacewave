import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
/* eslint-disable react-doctor/rerender-state-only-in-handlers */
import {
  useState,
  useRef,
  useEffect,
  useMemo,
  useCallback,
  type DragEvent,
  type KeyboardEvent,
} from 'react'
import { LuChevronRight, LuHouse } from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  motion as themeMotion,
  colors as themeColors,
} from '../../style/tokens.stylex.js'

const styles = stylex.create({
  rootIcon: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  chevronIcon: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-foreground-alt'],
  },
  editTarget: {
    height: '100%',
    minWidth: `calc(${themeLayout['--spacing']} * 2)`,
    flex: 1,
    cursor: 'text',
    alignSelf: 'stretch',
  },
  active: {
    fontSize: themeLayout['--text-ui'],
    lineHeight: 'var(--tw-leading, var(--text-ui--line-height))',
    letterSpacing: 'var(--tw-tracking, var(--text-ui--letter-spacing))',
  },
  editingBase: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 5)`,
    flex: 1,
    alignItems: 'center',
    borderRadius: '.25rem',
    backgroundColor: themeColors['--color-file-path-bar'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
  },
  inputBase: {
    width: '100%',
    backgroundColor: '#0000',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: themeColors['--color-foreground'],
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
  },
  breadcrumbsBase: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 5)`,
    flex: 1,
    alignItems: 'center',
    overflow: 'hidden',
    borderRadius: '.25rem',
    backgroundColor: {
      default: themeColors['--color-file-path-bar'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-file-path-bar-hover'],
      },
    },
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rootBase: {
    display: 'flex',
    alignItems: 'center',
    borderRadius: '.25rem',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-pulldown-hover'],
      },
    },
  },
  segmentBase: { alignItems: 'center' },
  crumbBase: {
    borderRadius: '.25rem',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-pulldown-hover'],
      },
    },
  },
  editingTouch: {
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  inputTouch: {
    fontSize: {
      default: themeFonts['--text-xs'],
      '@media (pointer: coarse)': 'var(--text-base)',
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
      '@media (pointer: coarse)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
  },
  input: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  breadcrumbsTouch: {
    minWidth: 0,
    gap: {
      default: `calc(${themeLayout['--spacing']} * .5)`,
      '@media (pointer: coarse)': 0,
    },
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      '@media (pointer: coarse)': 0,
    },
    fontSize: {
      default: themeFonts['--text-xs'],
      '@media (pointer: coarse)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
      '@media (pointer: coarse)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  breadcrumbs: {
    gap: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rootTouch: {
    paddingInline: {
      default: themeLayout['--spacing'],
      '@media (pointer: coarse)': 0,
    },
    width: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    height: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    flexShrink: { default: null, '@media (pointer: coarse)': 0 },
    justifyContent: { default: null, '@media (pointer: coarse)': 'center' },
  },
  root: { paddingInline: themeLayout['--spacing'] },
  lastSegmentTouch: {
    display: 'flex',
    minWidth: { default: null, '@media not all and (min-width: 40rem)': 0 },
    flex: { default: null, '@media not all and (min-width: 40rem)': 1 },
  },
  hiddenSegmentTouch: {
    display: {
      default: 'flex',
      '@media not all and (min-width: 40rem)': 'none',
    },
  },
  segment: { display: 'flex' },
  crumbTouch: {
    paddingInline: {
      default: themeLayout['--spacing'],
      '@media not all and (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 2)`,
    },
    whiteSpace: {
      default: 'nowrap',
      '@media not all and (min-width: 40rem)': 'nowrap',
    },
    minWidth: { default: null, '@media not all and (min-width: 40rem)': 0 },
    flex: { default: null, '@media not all and (min-width: 40rem)': 1 },
    textOverflow: {
      default: null,
      '@media not all and (min-width: 40rem)': 'ellipsis',
    },
    overflow: {
      default: null,
      '@media not all and (min-width: 40rem)': 'hidden',
    },
    textAlign: {
      default: null,
      '@media not all and (min-width: 40rem)': 'left',
    },
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  crumb: { paddingInline: themeLayout['--spacing'], whiteSpace: 'nowrap' },
})

interface PathInputProps {
  className?: never
  path: string
  onPathChange?: (path: string) => void
  onNavigate?: (path: string) => void
  onPathTargetDragOver?: (
    path: string,
    event: DragEvent<HTMLElement>,
  ) => boolean
  onPathTargetDrop?: (path: string, event: DragEvent<HTMLElement>) => void
  sx?: ComponentStyles
  mobileTouchTargets?: boolean
}

/** PathInput renders an editable path with breadcrumb navigation. */
export function PathInput({
  path,
  onPathChange,
  onNavigate,
  onPathTargetDragOver,
  onPathTargetDrop,
  sx,
  mobileTouchTargets = false,
}: PathInputProps) {
  // Keep the path draft scoped to the path currently being edited.
  const [isEditing, setIsEditing] = useState(false)
  const [editState, setEditState] = useState({ path, value: path })
  const inputRef = useRef<HTMLInputElement>(null)
  const editValue = editState.path === path ? editState.value : path
  const setEditValue = useCallback(
    (value: string) => setEditState({ path, value }),
    [path],
  )

  // Select the path text when its editor becomes active.
  useEffect(() => {
    if (isEditing && inputRef.current) {
      inputRef.current.focus()
      inputRef.current.select()
    }
  }, [isEditing])

  // Resolve the current path into navigable breadcrumb segments.
  const pathSegments = useMemo(() => path.split('/').filter(Boolean), [path])

  // Navigate to the ancestor represented by the selected breadcrumb.
  const handleBreadcrumbClick = useCallback(
    (index: number) => {
      const newPath = '/' + pathSegments.slice(0, index + 1).join('/')
      onNavigate?.(newPath)
    },
    [pathSegments, onNavigate],
  )

  // Navigate directly to the root path.
  const handleRootClick = useCallback(() => {
    onNavigate?.('/')
  }, [onNavigate])

  // Open the current path for editing.
  const handleEditClick = useCallback(() => {
    setIsEditing(true)
  }, [])

  // Commit a changed path when focus leaves its editor.
  const handleInputBlur = useCallback(() => {
    setIsEditing(false)
    if (editValue !== path) {
      onPathChange?.(editValue)
    }
  }, [editValue, path, onPathChange])

  // Commit with Enter or discard editing with Escape.
  const handleInputKeyDown = useCallback(
    (e: KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter') {
        setIsEditing(false)
        if (editValue !== path) {
          onPathChange?.(editValue)
        }
      }
      if (e.key === 'Escape') {
        setIsEditing(false)
        setEditState({ path, value: path })
      }
    },
    [editValue, path, onPathChange],
  )

  // Render the editable path or its navigation breadcrumbs.
  if (isEditing) {
    return (
      <div
        sx={[
          [styles.editingBase, mobileTouchTargets && styles.editingTouch],
          sx,
        ]}
      >
        <input
          ref={inputRef}
          type="text"
          aria-label="File path"
          value={editValue}
          onChange={(e) => setEditValue(e.target.value)}
          onBlur={handleInputBlur}
          onKeyDown={handleInputKeyDown}
          sx={[
            styles.inputBase,
            mobileTouchTargets ? styles.inputTouch : styles.input,
          ]}
          spellCheck={false}
          autoComplete="off"
        />
      </div>
    )
  }

  return (
    <div
      role="group"
      aria-label="File path"
      sx={[
        [
          styles.breadcrumbsBase,
          mobileTouchTargets ? styles.breadcrumbsTouch : styles.breadcrumbs,
        ],
        sx,
      ]}
    >
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation()
          handleRootClick()
        }}
        onDragOver={(e) => onPathTargetDragOver?.('/', e)}
        onDrop={(e) => onPathTargetDrop?.('/', e)}
        sx={[
          styles.rootBase,
          mobileTouchTargets ? styles.rootTouch : styles.root,
          pathSegments.length === 0 && styles.active,
        ]}
        aria-label="Navigate to root"
      >
        <LuHouse sx={styles.rootIcon} />
      </button>

      {pathSegments.map((segment, index) => (
        <div
          key={pathSegments.slice(0, index + 1).join('/')}
          sx={[
            styles.segmentBase,
            mobileTouchTargets
              ? index === pathSegments.length - 1
                ? styles.lastSegmentTouch
                : styles.hiddenSegmentTouch
              : styles.segment,
          ]}
        >
          <LuChevronRight sx={styles.chevronIcon} />
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation()
              handleBreadcrumbClick(index)
            }}
            onDragOver={(e) =>
              onPathTargetDragOver?.(
                '/' + pathSegments.slice(0, index + 1).join('/'),
                e,
              )
            }
            onDrop={(e) =>
              onPathTargetDrop?.(
                '/' + pathSegments.slice(0, index + 1).join('/'),
                e,
              )
            }
            sx={[
              styles.crumbBase,
              mobileTouchTargets ? styles.crumbTouch : styles.crumb,
              index === pathSegments.length - 1 && styles.active,
            ]}
            aria-label={`Navigate to ${segment}`}
          >
            {segment}
          </button>
        </div>
      ))}

      <button
        type="button"
        aria-label="Edit path"
        onClick={handleEditClick}
        sx={styles.editTarget}
      />
    </div>
  )
}
