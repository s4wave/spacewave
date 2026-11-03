import { useId } from 'react'
import {
  LuCopy,
  LuGitBranch,
  LuLayers,
  LuPlus,
} from '@s4wave/web/images/icons.js'

import type { ConfigEditorProps } from '@s4wave/web/configtype/configtype.js'
import { Input } from '@s4wave/web/ui/input.js'

import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import type { CreateGitRepoWizardOp } from '@s4wave/core/git/git.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule3: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule4: {
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule6: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule7: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
  rule8: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: '.25rem',
    accentColor: themeColors['--color-brand'],
  },
  rule9: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
})

// GitRepoConfigEditor edits the config-specific fields of a CreateGitRepoWizardOp.
// Renders a new/clone toggle and clone options (URL, ref, depth, recursive).
export function GitRepoConfigEditor({
  sx,
  value,
  onValueChange,
}: ConfigEditorProps<CreateGitRepoWizardOp> & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const cloneOpts = value.cloneOpts ?? {}
  const cloneUrlId = useId()
  const cloneRefId = useId()

  const handleSelectMode = (clone: boolean) => {
    onValueChange({ ...value, clone })
  }

  return (
    <div sx={[false, sx]}>
      <section sx={nativeChildSpacing.s3}>
        <div sx={stylexStyles2.rule1}>
          <h3 sx={stylexStyles2.rule2}>
            <LuGitBranch sx={stylexStyles.rule1} />
            Repository Source
          </h3>
        </div>
        <div sx={false}>
          <RadioOption
            sx={nativeChildSpacing.s2}
            selected={!value.clone}
            onSelect={() => handleSelectMode(false)}
            icon={<LuPlus sx={stylexStyles.rule1} />}
            label="New empty repository"
            description="Start with an empty Git repository."
          />
          <RadioOption
            sx={nativeChildSpacing.s2}
            selected={value.clone ?? false}
            onSelect={() => handleSelectMode(true)}
            icon={<LuCopy sx={stylexStyles.rule1} />}
            label="Clone a repository"
            description="Import history from an existing Git URL."
          />
        </div>
      </section>

      {value.clone && (
        <section sx={nativeChildSpacing.s3}>
          <div sx={stylexStyles2.rule1}>
            <h3 sx={stylexStyles2.rule2}>
              <LuCopy sx={stylexStyles.rule1} />
              Clone Options
            </h3>
          </div>
          <div sx={stylexStyles3.rule1}>
            <div sx={[nativeChildSpacingNext.s3, false]}>
              <label
                htmlFor={cloneUrlId}
                sx={[nativeChildSpacing.s2, stylexStyles2.rule3]}
              >
                Clone URL
              </label>
              <Input
                sx={nativeChildSpacing.s2}
                id={cloneUrlId}
                value={cloneOpts.url ?? ''}
                onChange={(e) =>
                  onValueChange({
                    ...value,
                    cloneOpts: { ...cloneOpts, url: e.target.value },
                  })
                }
                placeholder="https://github.com/user/repo.git"
                variant="configCompact"
              />
            </div>
            <div sx={[nativeChildSpacingNext.s3, false]}>
              <label
                htmlFor={cloneRefId}
                sx={[nativeChildSpacing.s2, stylexStyles2.rule3]}
              >
                Branch / Ref
              </label>
              <Input
                sx={nativeChildSpacing.s2}
                id={cloneRefId}
                value={cloneOpts.ref ?? ''}
                onChange={(e) =>
                  onValueChange({
                    ...value,
                    cloneOpts: { ...cloneOpts, ref: e.target.value },
                  })
                }
                placeholder="main (leave empty for default)"
                variant="configCompact"
              />
            </div>
            <div sx={[nativeChildSpacingNext.s3, false]}>
              <span sx={[nativeChildSpacing.s2, stylexStyles2.rule3]}>
                Depth
              </span>
              <div sx={[nativeChildSpacing.s2, stylexStyles2.rule4]}>
                <button
                  type="button"
                  sx={
                    (cloneOpts.depth ?? 0) === 0
                      ? stylexStyles2.rule5
                      : stylexStyles2.rule6
                  }
                  onClick={() =>
                    onValueChange({
                      ...value,
                      cloneOpts: { ...cloneOpts, depth: 0 },
                    })
                  }
                >
                  Full history
                </button>
                <button
                  type="button"
                  sx={
                    cloneOpts.depth === 1
                      ? stylexStyles2.rule5
                      : stylexStyles2.rule6
                  }
                  onClick={() =>
                    onValueChange({
                      ...value,
                      cloneOpts: { ...cloneOpts, depth: 1 },
                    })
                  }
                >
                  Shallow clone
                </button>
              </div>
            </div>
            <label sx={[nativeChildSpacingNext.s3, stylexStyles2.rule7]}>
              <input
                type="checkbox"
                checked={cloneOpts.recursive ?? false}
                onChange={(e) =>
                  onValueChange({
                    ...value,
                    cloneOpts: { ...cloneOpts, recursive: e.target.checked },
                  })
                }
                sx={stylexStyles2.rule8}
              />
              <span sx={stylexStyles2.rule9}>
                <LuLayers sx={stylexStyles.rule2} />
                Clone submodules recursively
              </span>
            </label>
          </div>
        </section>
      )}
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
