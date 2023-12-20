import React from 'react'

// AbortComponent extends React.PureComponent with an abortController that is canceled when unmounted.
//
// NOTE: It is recommended to use React Functional Components instead. See useAbortSignal.
export class AbortComponent<
  P = Record<string, never>,
  S = Record<string, never>,
  SS = unknown,
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
