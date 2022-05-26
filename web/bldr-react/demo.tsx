import React from 'react'

// Demo contains a bldr runtime and a web view.
export class Demo extends React.Component {
  public render() {
    // the /b/ path is controlled by the service worker.
    return <img src="/b/test.png" />
  }
}
