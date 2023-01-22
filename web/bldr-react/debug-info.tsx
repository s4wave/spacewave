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
        float: 'right',
        background: 'black',
        color: 'white',
        fontSize: '12px',
        padding: '5px',
        margin: '5px',
      }}
    >
      {props.children}
    </div>
  )
}
