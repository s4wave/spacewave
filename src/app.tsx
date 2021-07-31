import React from 'react'
import { ChakraProvider } from '@chakra-ui/react'
import { AppContainer, Runtime } from './bldr'
import Login from './login/Login'
import theme from './theme'

import '@fontsource/raleway/400.css'
import '@fontsource/open-sans/700.css'

interface IAppProps {
  // runtime is the external bldr runtime handle
  // if unset, constructs a default Runtime
  runtime?: Runtime
}

// App is the root entrypoint of the sandbox.
export class App extends React.Component<IAppProps> {
  private externalRuntime?: boolean
  private runtime: Runtime

  constructor(props: IAppProps) {
    super(props)
    if (props.runtime) {
      this.externalRuntime = true
      this.runtime = props.runtime
    } else {
      this.runtime = new Runtime()
    }
  }

  public componentWillUnmount() {
    if (this.runtime && !this.externalRuntime) {
      this.runtime.dispose()
    }
  }

  public render() {
    return (
      <div className="app">
        <AppContainer runtime={this.runtime}>
          <ChakraProvider theme={theme}>
            <Login />
          </ChakraProvider>
        </AppContainer>
      </div>
    )
  }
}
