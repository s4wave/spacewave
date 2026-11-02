import * as iconStylex from '@stylexjs/stylex'
import { IconStylesProvider } from '@s4wave/web/images/icons.js'
import {
  iconDefaults,
  buttonGroupIconMarker,
} from '../style/icon-defaults.stylex.js'
import { Slot } from '@radix-ui/react-slot'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@s4wave/web/style/utils.js'
import { Separator } from '@s4wave/web/ui/separator'
import * as stylex from '@stylexjs/stylex'
import { colors } from '../style/tokens.stylex.js'

const separatorStyles = stylex.create({
  base: {
    backgroundColor: colors['--color-input'],
    position: 'relative',
    margin: 0,
    alignSelf: 'stretch',
    height: {
      default: null,
      ':is([data-orientation="horizontal"])': '1px',
      ':is([data-orientation="vertical"])': 'auto',
    },
  },
})

const buttonGroupVariants = cva(
  "flex w-fit items-stretch has-[>[data-slot=button-group]]:gap-2 [&>*]:focus-visible:relative [&>*]:focus-visible:z-10 has-[select[aria-hidden=true]:last-child]:[&>[data-slot=select-trigger]:last-of-type]:rounded-r-md [&>[data-slot=select-trigger]:not([class*='w-'])]:w-fit [&>input]:flex-1",
  {
    variants: {
      orientation: {
        horizontal:
          '[&>*:not(:first-child)]:rounded-l-none [&>*:not(:first-child)]:border-l-0 [&>*:not(:last-child)]:rounded-r-none',
        vertical:
          'flex-col [&>*:not(:first-child)]:rounded-t-none [&>*:not(:first-child)]:border-t-0 [&>*:not(:last-child)]:rounded-b-none',
      },
    },
    defaultVariants: {
      orientation: 'horizontal',
    },
  },
)

function ButtonGroup({
  className,
  orientation,
  ...props
}: React.ComponentProps<'div'> & VariantProps<typeof buttonGroupVariants>) {
  return (
    <div
      role="group"
      data-slot="button-group"
      data-orientation={orientation}
      className={
        /* TODO(stylex): computed class helper */ cn(
          buttonGroupVariants({ orientation }),
          className,
        )
      }
      {...props}
    />
  )
}

function ButtonGroupText({
  className,
  asChild = false,
  ...props
}: React.ComponentProps<'div'> & {
  asChild?: boolean
}) {
  const Comp = asChild ? Slot : 'div'

  return (
    <IconStylesProvider sx={[iconDefaults.buttonGroup]}>
      <Comp
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: [&_svg]:pointer-events-none */ cn(
            'bg-muted flex items-center gap-2 rounded-md border px-4 text-sm font-medium shadow-xs [&_svg]:pointer-events-none',
            className,
            iconStylex.props(buttonGroupIconMarker).className,
          )
        }
        {...props}
      />
    </IconStylesProvider>
  )
}

function ButtonGroupSeparator({
  sx,
  orientation = 'vertical',
  ...props
}: React.ComponentProps<typeof Separator>) {
  return (
    <Separator
      data-slot="button-group-separator"
      orientation={orientation}
      sx={[separatorStyles.base, sx]}
      {...props}
    />
  )
}

export {
  ButtonGroup,
  ButtonGroupSeparator,
  ButtonGroupText,
  buttonGroupVariants,
}
