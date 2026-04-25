import React, { CSSProperties } from 'react'

import { Frame } from '@s4wave/web/frame/frame.js'
import { IBarProps } from '@s4wave/web/frame/bar.js'
import { AppLogo } from '@s4wave/web/images/AppLogo.js'
import { cn } from '@s4wave/web/style/utils.js'

// IWindowFrameProps are properties for the window frame.
export interface IWindowFrameProps {
  title?: string
  style?: CSSProperties
  className?: string
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

export function WindowFrame(props: IWindowFrameProps) {
  let header: React.ReactNode = (
    <>
      <AppLogo
        className={cn('select-none', !props.topBarHeight && 'max-h-bar')}
        style={
          props.topBarHeight ?
            { maxHeight: `${props.topBarHeight}px` }
          : undefined
        }
      />
      <span className="text-white/72 select-none">{props.title}</span>
    </>
  )
  if (props.centerTopBar) {
    header = <div className="mx-auto flex items-center text-xs">{header}</div>
  }

  const buttons: React.JSX.Element[] = []
  if (props.onClose) {
    buttons.push(
      <button
        key="close"
        title="Close Window"
        onClick={props.onClose}
        className="text-gray-500 hover:text-white focus:outline-none"
        style={{ fontSize: '22px' }}
      >
        &times;
      </button>,
    )
  }

  return (
    <Frame
      style={props.style}
      className={props.className}
      bottomBar={{ hidden: true, ...props.bottomBar }}
      innerClassName="flex-col"
      topBar={{
        className: cn(
          'dark bg-window-bar mb-px',
          !props.topBarHeight && 'h-bar',
          props.topBar?.className,
        ),
        style: {
          ...(props.topBarHeight && { height: `${props.topBarHeight}px` }),
          ...props.topBar?.style,
        },
        right: buttons,
        left: header,
        leftStyle: {
          WebkitAppRegion: 'drag',
          ...props.topBar?.leftStyle,
        },
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
