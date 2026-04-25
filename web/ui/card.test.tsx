import { describe, it, expect, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardAction,
  CardContent,
  CardFooter,
} from './card.js'

describe('Card', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders with data-slot="card" and children', () => {
    const { container } = render(<Card>Card body</Card>)
    const card = container.querySelector('[data-slot="card"]')
    expect(card).toBeTruthy()
    expect(screen.getByText('Card body')).toBeDefined()
  })

  it('accepts custom className', () => {
    const { container } = render(<Card className="my-card">Content</Card>)
    const card = container.querySelector('[data-slot="card"]')
    expect(card?.className).toContain('my-card')
  })
})

describe('CardHeader', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders with data-slot="card-header"', () => {
    const { container } = render(<CardHeader>Header</CardHeader>)
    const header = container.querySelector('[data-slot="card-header"]')
    expect(header).toBeTruthy()
    expect(screen.getByText('Header')).toBeDefined()
  })

  it('accepts custom className', () => {
    const { container } = render(
      <CardHeader className="custom-header">Header</CardHeader>,
    )
    const header = container.querySelector('[data-slot="card-header"]')
    expect(header?.className).toContain('custom-header')
  })
})

describe('CardTitle', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders with data-slot="card-title"', () => {
    const { container } = render(<CardTitle>Title</CardTitle>)
    const title = container.querySelector('[data-slot="card-title"]')
    expect(title).toBeTruthy()
    expect(screen.getByText('Title')).toBeDefined()
  })

  it('accepts custom className', () => {
    const { container } = render(
      <CardTitle className="custom-title">Title</CardTitle>,
    )
    const title = container.querySelector('[data-slot="card-title"]')
    expect(title?.className).toContain('custom-title')
  })
})

describe('CardDescription', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders with data-slot="card-description"', () => {
    const { container } = render(
      <CardDescription>Description text</CardDescription>,
    )
    const desc = container.querySelector('[data-slot="card-description"]')
    expect(desc).toBeTruthy()
    expect(screen.getByText('Description text')).toBeDefined()
  })

  it('accepts custom className', () => {
    const { container } = render(
      <CardDescription className="custom-desc">Desc</CardDescription>,
    )
    const desc = container.querySelector('[data-slot="card-description"]')
    expect(desc?.className).toContain('custom-desc')
  })
})

describe('CardAction', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders with data-slot="card-action"', () => {
    const { container } = render(<CardAction>Action</CardAction>)
    const action = container.querySelector('[data-slot="card-action"]')
    expect(action).toBeTruthy()
    expect(screen.getByText('Action')).toBeDefined()
  })

  it('accepts custom className', () => {
    const { container } = render(
      <CardAction className="custom-action">Action</CardAction>,
    )
    const action = container.querySelector('[data-slot="card-action"]')
    expect(action?.className).toContain('custom-action')
  })
})

describe('CardContent', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders with data-slot="card-content"', () => {
    const { container } = render(<CardContent>Content here</CardContent>)
    const content = container.querySelector('[data-slot="card-content"]')
    expect(content).toBeTruthy()
    expect(screen.getByText('Content here')).toBeDefined()
  })

  it('accepts custom className', () => {
    const { container } = render(
      <CardContent className="custom-content">Content</CardContent>,
    )
    const content = container.querySelector('[data-slot="card-content"]')
    expect(content?.className).toContain('custom-content')
  })
})

describe('CardFooter', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders with data-slot="card-footer"', () => {
    const { container } = render(<CardFooter>Footer</CardFooter>)
    const footer = container.querySelector('[data-slot="card-footer"]')
    expect(footer).toBeTruthy()
    expect(screen.getByText('Footer')).toBeDefined()
  })

  it('accepts custom className', () => {
    const { container } = render(
      <CardFooter className="custom-footer">Footer</CardFooter>,
    )
    const footer = container.querySelector('[data-slot="card-footer"]')
    expect(footer?.className).toContain('custom-footer')
  })
})

describe('Card composition', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders all subcomponents together', () => {
    const { container } = render(
      <Card>
        <CardHeader>
          <CardTitle>My Card</CardTitle>
          <CardDescription>A description</CardDescription>
          <CardAction>Action button</CardAction>
        </CardHeader>
        <CardContent>Main content</CardContent>
        <CardFooter>Footer content</CardFooter>
      </Card>,
    )

    expect(container.querySelector('[data-slot="card"]')).toBeTruthy()
    expect(container.querySelector('[data-slot="card-header"]')).toBeTruthy()
    expect(container.querySelector('[data-slot="card-title"]')).toBeTruthy()
    expect(
      container.querySelector('[data-slot="card-description"]'),
    ).toBeTruthy()
    expect(container.querySelector('[data-slot="card-action"]')).toBeTruthy()
    expect(container.querySelector('[data-slot="card-content"]')).toBeTruthy()
    expect(container.querySelector('[data-slot="card-footer"]')).toBeTruthy()

    expect(screen.getByText('My Card')).toBeDefined()
    expect(screen.getByText('A description')).toBeDefined()
    expect(screen.getByText('Action button')).toBeDefined()
    expect(screen.getByText('Main content')).toBeDefined()
    expect(screen.getByText('Footer content')).toBeDefined()
  })
})
