import React from 'react'

import type { Runtime } from './runtime'

interface IDebugProps {
  // runtime is the app runtime
  runtime: Runtime
}

// Debug is the debug component for bldr app.
export class Debug extends React.Component<IDebugProps> {
  public render() {
    return (
      <div className="bldr-debug">
        Loaded bldr debug:
        {' ' + JSON.stringify(this.props.runtime)}
      </div>
    )
  }
}
