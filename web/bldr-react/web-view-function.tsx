import React from 'react'
import { castToError } from '../bldr/error.js'
import { constantBackoff, retryWithAbort } from '../bldr/retry.js'
import { BldrContext } from './bldr-context.js'
import { FunctionComponent } from './function-component.js'

// IFunctionComponentContainerProps are props for FunctionComponentContainer.
export interface IFunctionComponentContainerProps {
  // scriptPath is the function component script path to render.
  scriptPath: string
  // componentProps is an optional props message to the component.
  componentProps?: Uint8Array
}

// IFunctionComponentContainerState is state for FunctionComponentContainer.
interface IFunctionComponentContainerState {
  // loadError is an error caught while loading the script.
  loadError?: Error
}

// FunctionComponentContainer imports and initializes a FunctionComponent script.
export class FunctionComponentContainer extends React.PureComponent<
  IFunctionComponentContainerProps,
  IFunctionComponentContainerState
> {
  // context is the webDocument context
  declare context: React.ContextType<typeof BldrContext>
  static contextType = BldrContext

  // closeController is aborted when the component is unmounted.
  private closeController: AbortController
  // scriptPath is the path to the script to render.
  private scriptPath: string
  // divRef is the ref to the parent div for the function component.
  private divRef?: HTMLDivElement
  // functionComponent is the imported function component.
  private functionComponent?: FunctionComponent
  // functionComponentRelease releases the instantiated function component.
  private functionComponentRelease?: () => void

  constructor(props: IFunctionComponentContainerProps) {
    super(props)
    this.scriptPath = ''
    this.state = {}
    this.closeController = new AbortController()
  }

  public componentDidMount() {
    this.setScriptPath(this.props.scriptPath)
  }

  // setScriptPath sets the script path.
  public setScriptPath(scriptPath: string) {
    if (scriptPath === this.scriptPath) {
      return
    }
    this.scriptPath = scriptPath
    if (scriptPath.length === 0) {
      this.update(undefined, this.divRef)
      return
    }
    retryWithAbort(
      this.closeController.signal,
      async () => {
        const script = await import(this.scriptPath)
        let functionComponent: FunctionComponent | undefined = undefined
        if (script?.default && typeof script.default === 'function') {
          functionComponent = script.default as FunctionComponent
        } else {
          console.error(
            'expected default exported function for script',
            this.scriptPath,
            script.default
          )
          throw new Error(
            'expected default exported function for script: ' + this.scriptPath
          )
        }
        this.setState({ loadError: undefined })
        this.update(functionComponent, this.divRef)
      },
      {
        backoffFn: constantBackoff(1000),
        errorCb: (err) => {
          this.setState({
            loadError: castToError(
              err,
              'error loading script: ' + this.scriptPath
            ),
          })
        },
      }
    )
  }

  public componentWillUnmount() {
    this.closeController.abort()
    this.update(this.functionComponent, undefined)
  }

  public render() {
    return this.state.loadError ? (
      <>
        Error: {this.state.loadError.message}
        <br />
      </>
    ) : (
      <div
        style={{
          width: '100%',
          height: '100%',
          position: 'relative',
          overflow: 'hidden',
        }}
        ref={(ref) => this.update(this.functionComponent, ref || undefined)}
      />
    )
  }

  // update updates the function component and/or div-ref field.
  private update(functionComponent?: FunctionComponent, ref?: HTMLDivElement) {
    if (this.functionComponent === functionComponent && this.divRef === ref) {
      // no changes
      return
    }
    if (this.functionComponentRelease) {
      this.functionComponentRelease()
      delete this.functionComponentRelease
    }
    this.divRef = ref
    this.functionComponent = functionComponent
    if (this.functionComponent && this.divRef && this.context) {
      this.functionComponentRelease = this.functionComponent(
        this.context,
        this.divRef,
        this.props.componentProps
      )
    }
  }
}
