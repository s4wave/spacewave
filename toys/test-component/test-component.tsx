import React from 'react'

// Demo is a minimum-viable test component.
export default class Demo extends React.Component {
  public render() {
    // the /b/ path is controlled by the service worker.
    return (
      <>
        <span>Hello from the dynamically imported Demo component!</span>
        <br />
        <img src="/b/test.png" />
      </>
    )
  }
}
