import * as $ from 'jquery'
import * as React from 'react'

import './index.css'

import Thing from './thing'

export interface ILayoutState {
  testState?: string
}

export interface ILayoutProps {
  testProp?: string
}

export default class Layout extends React.Component<
  ILayoutProps,
  ILayoutState
> {
  constructor(props: ILayoutProps) {
    super(props)

    let thing = new Thing();
    thing.testProp = "testing";
    this.state = {
      testState: thing.testProp,
    }
  }

  public render() {
    return (
      <div className="layout layout-test" />
    )
  }
}
