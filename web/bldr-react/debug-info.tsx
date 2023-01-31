import React from 'react'

// IDebugInfoProps are props for DebugInfo.
interface IDebugInfoProps {
  // children are children elements for the debug
  children?: React.ReactNode
}

// DebugInfo displays debug information in the top right corner.
export function DebugInfo(props: IDebugInfoProps) {
  return (
    <div
      style={{
        position: 'absolute',
        zIndex: 10,
        right: '5px',
        background: 'rgba(0, 0, 0, 0.72)',
        color: 'white',
        fontSize: '12px',
        padding: '5px',
        margin: '5px',
        maxWidth: '33%',
        minWidth: '250px',
        overflow: 'hidden',
        overflowWrap: 'break-word',
      }}
    >
      {props.children}
    </div>
  )
}
