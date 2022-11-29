import React from 'react'
import { createRoot } from 'react-dom/client'

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

export default function(parent: HTMLDivElement): (() => void) {
    const root = createRoot(parent)
    root.render(<Example />)
    return root.unmount.bind(root)
}

