import { LuTriangleAlert } from '@s4wave/web/images/icons.js'
import * as stylex from '@stylexjs/stylex'

import { colors, fonts, layout, radii } from '../../web/style/tokens.stylex.js'

const styles = stylex.create({
  banner: {
    borderColor: `color-mix(in oklab, ${colors['--color-destructive']} 20%, transparent)`,
    borderRadius: radii['--radius-md'],
    borderStyle: 'solid',
    borderWidth: 1,
    gap: `calc(${layout['--spacing']} * 2)`,
    paddingBlock: `calc(${layout['--spacing']} * 2)`,
    paddingInline: `calc(${layout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    backgroundColor: `color-mix(in oklab, ${colors['--color-destructive']} 5%, transparent)`,
    color: colors['--color-destructive'],
    display: 'flex',
    fontSize: fonts['--text-xs'],
    lineHeight: fonts['--text-xs--line-height'],
  },
  icon: {
    flexShrink: 0,
    height: `calc(${layout['--spacing']} * 3.5)`,
    marginTop: 1,
    width: `calc(${layout['--spacing']} * 3.5)`,
  },
  content: { flexBasis: '0%', flexGrow: 1, flexShrink: 1, minWidth: 0 },
  title: { fontWeight: fonts['--font-weight-medium'] },
  message: {
    color: colors['--color-foreground-alt'],
    overflowWrap: 'break-word',
  },
})

/** VmErrorBannerProps configures an error notice. */
export interface VmErrorBannerProps {
  title: string
  message: string
}

/** VmErrorBanner states a VM or request failure in a red notice. */
export function VmErrorBanner({ title, message }: VmErrorBannerProps) {
  return (
    <div role="alert" sx={styles.banner}>
      <LuTriangleAlert sx={styles.icon} />
      <div sx={styles.content}>
        <div sx={styles.title}>{title}</div>
        <div sx={styles.message}>{message}</div>
      </div>
    </div>
  )
}
