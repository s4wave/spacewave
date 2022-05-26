import React from 'react'

interface IDemoProps {}

interface IDemoState {
  showImage?: boolean
}

// Demo contains a bldr runtime and a web view.
export class Demo extends React.Component<IDemoProps, IDemoState> {
  constructor(props: IDemoProps) {
    super(props)
    this.state = {}
  }

  public componentDidMount() {
    setTimeout(() => {
      this.setState({ showImage: true })
    }, 3000)
  }

  public render() {
    if (!this.state.showImage) {
      return undefined
    }

    // the /b/ path is controlled by the service worker.
    return <img src="/b/test.png" />
  }
}
