import React from 'react'

import { BldrContext } from '@bldr/web/bldr-react/bldr-context.js'
import { EchoerClientImpl } from '@go/github.com/aperturerobotics/starpc/echo/index.js'
import { WebDocument as BldrWebDocument } from '@bldr/web/bldr/web-document.js'
import { WebView as BldrWebView } from '@bldr/web/bldr/web-view.js'
import { createFunctionComponent } from '@bldr/web/bldr-react/function-component.js'

import './example.css';

// IExampleState contains state for Example.
interface IExampleState {
    message?: string
}

class Example extends React.Component<{}, IExampleState> {
    // context is the webDocument context
    declare context: React.ContextType<typeof BldrContext>
    static contextType = BldrContext

    // echoHost is the echo service running on the plugin host.
    private echoHost?: EchoerClientImpl

    constructor(props: {}) {
        super(props)
        this.state = {}
    }

    // webDocument exposes the web document from context.
    get webDocument(): BldrWebDocument {
        return this.context!.webDocument!
    }

    // webView exposes the web view from context.
    get webView(): BldrWebView {
        return this.context!.webView!
    }

    public componentDidMount() {
        this.echoHost = new EchoerClientImpl(
            this.webDocument.buildWebViewHostClient(this.webView.getUuid())
        )
        this._runEchoRpc()
    }

    // _runEchoRpc runs the echo rpc and updates the state.
    private async _runEchoRpc(): Promise<void> {
        const resp = await this.echoHost?.Echo({ body: 'Hello world via RPC round-trip to example plugin!' })
        this.setState({message: resp?.body})
    }

    public render() {
        return (
            <div className='example-message'>
                {this.state.message}
            </div>
        )
    }
}

// Example will be constructed when the component is loaded.
export default createFunctionComponent(<Example />)
