import React from 'react'

const message = 'Hello world from Example Component'

export default class Example extends React.Component {
    public render() {
        return (
            <span>
                {message}
            </span>
        )
    }
}
