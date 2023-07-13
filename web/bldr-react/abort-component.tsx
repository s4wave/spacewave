import React from 'react'

// AbortComponent extends React.PureComponent with an abortController that is canceled when unmounted.
export class AbortComponent<
  P = {},
  S = {},
  SS = any,
> extends React.PureComponent<P, S, SS> {
  // abortController is aborted when the component is unmounted.
  protected abortController: AbortController

  constructor(props: P) {
    super(props)
    this.abortController = new AbortController()
  }

  public componentWillUnmount() {
    this.abortController.abort()
  }
}
