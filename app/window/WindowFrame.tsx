import * as frameStyles from '@stylexjs/stylex'
import React, { CSSProperties } from 'react'

import { Frame } from '@s4wave/web/frame/frame.js'
import { IBarProps } from '@s4wave/web/frame/bar.js'
import { AppLogo } from '@s4wave/web/images/AppLogo.js'

import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = frameStyles.create({
  rule1: {
    WebkitUserSelect: 'none',
    color: `color-mix(in oklab, ${themeColors['--color-white']} 72%, transparent)`,
    userSelect: 'none',
  },
  rule2: {
    marginInline: 'auto',
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule3: {
    '--tw-outline-style': { default: null, ':focus': 'none' },
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    outlineStyle: { default: null, ':focus': 'none' },
  },
  rule4: { flex: 1, alignItems: 'center', display: 'flex' },
})

const stylexStyles = frameStyles.create({
  rule1: { WebkitUserSelect: 'none', userSelect: 'none' },
  rule2: { maxHeight: themeLayout['--spacing-bar'] },
  rule3: { maxHeight: 'var(--window-topbar-height)' },
})

const frameBodyStyles = frameStyles.create({
  column: { flexDirection: 'column' },
  topBar: { backgroundColor: 'var(--color-window-bar)', marginBottom: '1px' },
})

// IWindowFrameProps are properties for the window frame.
export interface IWindowFrameProps {
  dark?: boolean
  title?: string
  style?: CSSProperties
  sx?: frameStyles.StyleXStyles
  className?: never
  topBarHeight?: number

  onMinimize?: (e: React.MouseEvent) => void
  onMaximize?: (e: React.MouseEvent) => void
  onClose?: (e: React.MouseEvent) => void

  // children is the children render function
  children?: React.ReactNode | React.JSX.Element

  // topBar contains overrides for the top bar
  topBar?: IBarProps
  // bottomBar contains overrides for the top bar
  bottomBar?: IBarProps

  // centerTopBar puts the top bar header on the center
  centerTopBar?: boolean
}

// WindowFrame composes the application window header and body.
export function WindowFrame({ sx, ...props }: IWindowFrameProps) {
  // Compose the title and logo before applying optional centering.
  let header: React.ReactNode = (
    <>
      <AppLogo
        sx={[
          stylexStyles.rule1,
          !props.topBarHeight && stylexStyles.rule2,
          !!props.topBarHeight && stylexStyles.rule3,
        ]}
        topbarHeight={props.topBarHeight}
      />
      <span sx={stylexStyles2.rule1}>{props.title}</span>
    </>
  )
  if (props.centerTopBar) {
    header = <div sx={stylexStyles2.rule2}>{header}</div>
  }

  // Show only the window actions supplied by this platform.
  const buttons: React.JSX.Element[] = []
  if (props.onClose) {
    buttons.push(
      <button
        type="button"
        key="close"
        title="Close Window"
        onClick={props.onClose}
        sx={stylexStyles2.rule3}
      >
        &times;
      </button>,
    )
  }

  return (
    <Frame
      style={props.style}
      sx={sx}
      dark={props.dark}
      bottomBar={{ hidden: true, ...props.bottomBar }}
      innerSx={frameBodyStyles.column}
      topBar={{
        dark: true,
        sx: [frameBodyStyles.topBar, props.topBar?.sx],
        style: {
          ...(props.topBarHeight && { height: `${props.topBarHeight}px` }),
          ...props.topBar?.style,
        },
        right: buttons,
        left: (
          <div sx={stylexStyles2.rule4} data-window-drag-region="true">
            {header}
          </div>
        ),
        ...props.topBar,
      }}
    >
      {props.children}
    </Frame>
  )
}

/*
  <button
    onClick={this.handleMinimize}
    className="window-frame-button"> &#x2012;
  </button>
  <button
    onClick={this.handleMaximize}
    className="window-frame-button">
      &#9633;
  </button>
*/
