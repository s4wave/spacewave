import { useCallback, useState } from 'react'
import {
  LuX,
  LuEye,
  LuInfo,
  LuDownload,
  LuTrash2,
} from '@s4wave/web/images/icons.js'
import { RxCube } from '@s4wave/web/images/icons.js'

import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { downloadURL } from '@s4wave/web/download.js'

import { toast } from '@s4wave/web/ui/toaster.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

import type { ObjectViewerComponent } from './object.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  radii as themeRadii,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

import { groupMarker } from '../style/relations.stylex.js'

import { groupMarker as groupMarker2 } from '../style/relations.stylex.js'

const stylexStyles5 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    cursor: 'pointer',
    display: 'flex',
    justifyContent: 'space-between',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule2: {
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    cursor: 'pointer',
    display: 'flex',
    justifyContent: 'space-between',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
})

const stylexStyles4 = stylex.create({ rule1: { minWidth: 0 } })

const childSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      },
    },
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
    cursor: 'pointer',
    display: 'flex',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    overflow: 'auto',
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
    minWidth: 0,
  },
  rule4: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    '--tw-tracking': 'var(--tracking-tight)',
    overflow: 'hidden',
    letterSpacing: 'var(--tracking-tight)',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule6: { color: themeColors['--color-foreground-alt'], flexShrink: 0 },
  rule7: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule8: {
    flex: 1,
    overflow: 'auto',
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule9: {
    marginInline: 'auto',
    maxWidth: 'var(--container-5xl)',
    width: '100%',
  },
  rule10: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 64rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule11: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule12: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule13: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule14: { flex: 1, minWidth: 0 },
  rule15: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    minWidth: 0,
  },
  rule16: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule17: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
    flexShrink: 0,
    fontSize: '.65rem',
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: 1.5,
    userSelect: 'none',
  },
  rule18: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    WebkitUserSelect: 'none',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: '.6rem',
    lineHeight: 1.5,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    userSelect: 'none',
    wordBreak: 'break-all',
  },
  rule19: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    color: themeColors['--color-brand'],
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    width: '100%',
  },
  rule20: {
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    cursor: { default: 'pointer', ':disabled': 'default' },
    display: 'flex',
    opacity: { default: null, ':disabled': 1 },
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 14)`,
    width: '100%',
  },
  rule21: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule22: {
    color: themeColors['--color-foreground'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule23: { flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 },
  rule24: {
    '--tw-leading': 'var(--leading-relaxed)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    userSelect: 'none',
  },
  rule25: {
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'space-between',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule26: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-destructive'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule27: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: themeRadii['--radius-md'],
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
    color: themeColors['--color-destructive'],
    flexShrink: 0,
    fontWeight: themeFonts['--font-weight-medium'],
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule28: {
    color: themeColors['--color-destructive'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule29: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule30: {
    WebkitUserSelect: 'none',
    color: `color-mix(in oklab, ${themeColors['--color-destructive']} 80%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule31: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule32: {
    borderRadius: themeRadii['--radius-md'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule33: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      },
    },
    color: themeColors['--color-destructive'],
    cursor: { default: null, ':disabled': 'not-allowed' },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

export interface ObjectViewerDetailsProps {
  objectKey: string
  typeID: string
  rootRef?: string
  exportUrl?: string
  availableComponents: ObjectViewerComponent[]
  selectedComponent?: ObjectViewerComponent
  missingComponentID?: string
  onComponentSelect: (component: ObjectViewerComponent) => void
  onCloseClick?: () => void
  onDeleteConfirm?: () => Promise<void>
}

// ObjectViewerDetails displays metadata and viewer selection for an object.
export function ObjectViewerDetails({
  sx,
  objectKey,
  typeID,
  rootRef,
  exportUrl,
  availableComponents,
  selectedComponent,
  missingComponentID,
  onComponentSelect,
  onCloseClick,
  onDeleteConfirm,
}: ObjectViewerDetailsProps & { sx?: stylex.StyleXStyles; className?: never }) {
  // Retain local disclosure and the pending viewer selection.
  const [technicalDetailsOpen, setTechnicalDetailsOpen] = useState(false)
  const [selectedComponentOverride, setSelectedComponentOverride] = useState<
    string | undefined
  >()

  // Keep the chosen viewer visible until its owner publishes the selection.
  const displayedSelectedComponentID =
    selectedComponentOverride ?? selectedComponent?.componentID
  const displayedSelectedComponent =
    availableComponents.find(
      (component) => component.componentID === displayedSelectedComponentID,
    ) ?? selectedComponent

  // Publish the selected viewer while retaining its immediate local presentation.
  const handleComponentSelect = useCallback(
    (component: ObjectViewerComponent) => {
      setSelectedComponentOverride(component.componentID)
      onComponentSelect(component)
    },
    [onComponentSelect],
  )

  return (
    <div sx={[stylexStyles.rule1, sx]}>
      <div sx={stylexStyles.rule2}>
        <div sx={stylexStyles.rule3}>
          <RxCube sx={stylexStyles.rule4} />
          <span sx={stylexStyles.rule5}>{objectKey}</span>
          <span sx={stylexStyles.rule6}>· Object</span>
        </div>
        {onCloseClick && (
          <Tooltip>
            <TooltipTrigger asChild>
              <DashboardButton
                icon={<LuX sx={stylexStyles.rule7} />}
                onClick={onCloseClick}
              />
            </TooltipTrigger>
            <TooltipContent side="bottom">Close</TooltipContent>
          </Tooltip>
        )}
      </div>

      <div sx={stylexStyles.rule8}>
        <div sx={stylexStyles.rule9}>
          <div sx={stylexStyles.rule10}>
            <div sx={stylexStyles4.rule1}>
              <section sx={childSpacing.s3}>
                <div sx={stylexStyles.rule11}>
                  <h2 sx={stylexStyles.rule12}>
                    <LuInfo sx={stylexStyles.rule13} />
                    About this object
                  </h2>
                </div>

                <InfoCard>
                  <div>
                    <CopyableField
                      sx={childSpacing.s3}
                      label="Object key"
                      value={objectKey}
                    />
                    {missingComponentID && (
                      <CopyableField
                        sx={childSpacing.s3}
                        label="Missing Component ID"
                        value={missingComponentID}
                      />
                    )}
                  </div>
                </InfoCard>
              </section>

              <CollapsibleSection
                sx={childSpacing.s3}
                title="Technical details"
                icon={<LuInfo sx={stylexStyles.rule13} />}
                open={technicalDetailsOpen}
                onOpenChange={setTechnicalDetailsOpen}
              >
                <div>
                  <CopyableField
                    sx={childSpacing.s2}
                    label="Type ID"
                    value={typeID}
                  />
                  {rootRef && (
                    <CopyableField
                      sx={childSpacing.s2}
                      label="Root Ref"
                      value={rootRef}
                    />
                  )}
                </div>
              </CollapsibleSection>
            </div>

            <div sx={stylexStyles4.rule1}>
              {exportUrl && (
                <ExportDataSection sx={childSpacing.s3} exportUrl={exportUrl} />
              )}

              {availableComponents.length > 0 && (
                <section sx={childSpacing.s3}>
                  <div sx={stylexStyles.rule11}>
                    <h2 sx={stylexStyles.rule12}>
                      <LuEye sx={stylexStyles.rule13} />
                      Viewer
                    </h2>
                  </div>

                  <InfoCard>
                    <div>
                      {availableComponents.map((component) => {
                        const isSelected =
                          displayedSelectedComponentID === component.componentID
                        return (
                          <button
                            key={component.componentID}
                            type="button"
                            aria-pressed={isSelected}
                            onClick={() => handleComponentSelect(component)}
                            sx={[
                              isSelected
                                ? stylexStyles5.rule1
                                : stylexStyles5.rule2,
                              childSpacing.s1_5,
                            ]}
                          >
                            <div sx={stylexStyles.rule14}>
                              <div sx={stylexStyles.rule15}>
                                <p sx={stylexStyles.rule16}>{component.name}</p>
                                {isSelected && (
                                  <span sx={stylexStyles.rule17}>Active</span>
                                )}
                              </div>
                              <p sx={stylexStyles.rule18}>
                                ID: {component.componentID}
                              </p>
                            </div>
                          </button>
                        )
                      })}
                    </div>

                    {displayedSelectedComponent && onCloseClick && (
                      <button
                        type="button"
                        onClick={onCloseClick}
                        sx={stylexStyles.rule19}
                      >
                        <LuEye sx={stylexStyles.rule13} />
                        Open viewer
                      </button>
                    )}
                  </InfoCard>
                </section>
              )}

              {onDeleteConfirm && (
                <DangerZoneSection
                  sx={childSpacing.s3}
                  objectKey={objectKey}
                  onDeleteConfirm={onDeleteConfirm}
                />
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

// ExportDataSection renders a download button for exporting object data.
function ExportDataSection({
  exportUrl,
  sx,
}: {
  exportUrl: string
  sx?: stylex.StyleXStyles
}) {
  const [status, setStatus] = useState<'idle' | 'busy' | 'success' | 'failure'>(
    'idle',
  )

  const handleExport = useCallback(async () => {
    if (status === 'busy') return

    setStatus('busy')
    try {
      await downloadURL(exportUrl)
      setStatus('success')
    } catch (err: unknown) {
      console.error('failed to export object data', err)
      setStatus('failure')
      toast.error('Export could not be prepared')
    }
  }, [exportUrl, status])

  return (
    <section sx={sx}>
      <div sx={stylexStyles.rule11}>
        <h2 sx={stylexStyles.rule12}>
          <LuDownload sx={stylexStyles.rule13} />
          Data
        </h2>
      </div>

      <div>
        <button
          type="button"
          onClick={() => void handleExport()}
          disabled={status === 'busy' || status === 'success'}
          aria-busy={status === 'busy'}
          sx={[stylexStyles.rule20, childSpacing.s2]}
        >
          <div sx={stylexStyles.rule21}>
            <LuDownload sx={stylexStyles.rule22} />
          </div>
          <div sx={stylexStyles.rule23}>
            <h4 sx={stylexStyles.rule16}>
              {status === 'busy'
                ? 'Preparing export…'
                : status === 'success'
                  ? 'Export ready'
                  : status === 'failure'
                    ? 'Export could not be prepared'
                    : 'Export Data'}
            </h4>
            <p sx={stylexStyles.rule24}>
              {status === 'busy'
                ? 'Preparing a downloadable archive…'
                : status === 'success'
                  ? 'The object contents are ready to download.'
                  : status === 'failure'
                    ? 'Try again to prepare the export.'
                    : 'Download object contents as zip'}
            </p>
          </div>
        </button>

        {status === 'failure' && (
          <div sx={[stylexStyles.rule25, childSpacing.s2]}>
            <p sx={stylexStyles.rule26}>Export could not be prepared.</p>
            <button
              type="button"
              onClick={() => void handleExport()}
              sx={stylexStyles.rule27}
            >
              Try again
            </button>
          </div>
        )}
      </div>
    </section>
  )
}

interface DangerZoneSectionProps {
  objectKey: string
  onDeleteConfirm: () => Promise<void>
  sx?: stylex.StyleXStyles
}

function DangerZoneSection({
  objectKey,
  onDeleteConfirm,
  sx,
}: DangerZoneSectionProps) {
  // Retain the destructive-action disclosure and its current submission feedback.
  const [open, setOpen] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string>()

  // Clear stale feedback when the confirmation dialog closes.
  const handleDialogOpenChange = useCallback((next: boolean) => {
    if (!next) {
      setError(undefined)
      setSubmitting(false)
    }
    setDialogOpen(next)
  }, [])

  // Invoke the existing deletion owner and retain a retryable failure state.
  const handleDelete = useCallback(async () => {
    setSubmitting(true)
    setError(undefined)
    try {
      await onDeleteConfirm()
      handleDialogOpenChange(false)
    } catch (err) {
      console.error('failed to delete object', err)
      setError('Delete could not be completed. Try again.')
      setSubmitting(false)
    }
  }, [handleDialogOpenChange, onDeleteConfirm])

  return (
    <>
      <CollapsibleSection
        sx={sx}
        title="Danger Zone"
        open={open}
        onOpenChange={setOpen}
      >
        <button
          type="button"
          onClick={() => setDialogOpen(true)}
          sx={[stylexStyles2.rule1, groupMarker]}
        >
          <div sx={stylexStyles3.rule1}>
            <LuTrash2 sx={stylexStyles.rule28} />
          </div>
          <div sx={stylexStyles.rule23}>
            <h4 sx={stylexStyles.rule29}>Delete Object</h4>
            <p sx={stylexStyles.rule30}>
              Permanently remove this object and all its data
            </p>
          </div>
        </button>
      </CollapsibleSection>

      <Dialog open={dialogOpen} onOpenChange={handleDialogOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Object</DialogTitle>
            <DialogDescription>
              This will permanently delete &ldquo;{objectKey}&rdquo; and all its
              data.
            </DialogDescription>
          </DialogHeader>

          {error && <p sx={stylexStyles.rule31}>{error}</p>}

          <DialogFooter>
            <button
              type="button"
              onClick={() => handleDialogOpenChange(false)}
              disabled={submitting}
              sx={stylexStyles.rule32}
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => void handleDelete()}
              disabled={submitting}
              sx={[stylexStyles.rule33]}
            >
              {submitting ? 'Deleting…' : 'Delete Object'}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
