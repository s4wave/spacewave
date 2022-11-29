import React from 'react'
import { createFunctionComponent } from 'web/bldr-react/function-component'

const message = 'Hello world from Example Component'

class Example extends React.Component {
    public render() {
        return (
            <span>
                {message}
            </span>
        )
    }
}

// Example will be constructed when the component is loaded.
export default createFunctionComponent(<Example />)
