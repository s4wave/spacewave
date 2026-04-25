import React from 'react'
import { describe, it, expect, afterEach } from 'vitest'
import { render, fireEvent, screen, cleanup } from '@testing-library/react'
import {
  StateNamespaceProvider,
  useStateAtom,
  useStateNamespace,
  useStateReducerAtom,
  atom,
  useParentStateNamespace,
} from './persist.js'

// Test component that uses the namespace state
function TestCounter({ namespace }: { namespace?: string[] }) {
  return (
    <StateNamespaceProvider namespace={namespace}>
      <CounterContent />
    </StateNamespaceProvider>
  )
}

function CounterContent() {
  const [count, setCount] = useStateAtom(null, 'count', 0)
  const { namespace: contextNamespace } = useParentStateNamespace()
  const testId =
    contextNamespace.length > 0 ?
      `counter-${contextNamespace.join('-')}`
    : 'counter-root'
  return (
    <button onClick={() => setCount((c: number) => c + 1)} data-testid={testId}>
      Count: {count}
    </button>
  )
}

describe('StateNamespaceProvider', () => {
  afterEach(() => {
    cleanup()
  })

  it('maintains isolated state for different namespaces', () => {
    const rootAtom = atom<Record<string, unknown>>({})

    render(
      <StateNamespaceProvider rootAtom={rootAtom} namespace={['test1']}>
        <TestCounter />
        <TestCounter namespace={['namespace1']} />
        <TestCounter namespace={['namespace2']} />
      </StateNamespaceProvider>,
    )

    const rootButton = screen.getByTestId('counter-test1')
    const ns1Button = screen.getByTestId('counter-test1-namespace1')
    const ns2Button = screen.getByTestId('counter-test1-namespace2')

    // Click root counter
    fireEvent.click(rootButton)
    expect(rootButton.textContent).toBe('Count: 1')
    expect(ns1Button.textContent).toBe('Count: 0')
    expect(ns2Button.textContent).toBe('Count: 0')

    // Click namespace1 counter
    fireEvent.click(ns1Button)
    expect(rootButton.textContent).toBe('Count: 1')
    expect(ns1Button.textContent).toBe('Count: 1')
    expect(ns2Button.textContent).toBe('Count: 0')

    // Click namespace2 counter
    fireEvent.click(ns2Button)
    expect(rootButton.textContent).toBe('Count: 1')
    expect(ns1Button.textContent).toBe('Count: 1')
    expect(ns2Button.textContent).toBe('Count: 1')
  })

  it('handles nested namespaces correctly', () => {
    const rootAtom = atom<Record<string, unknown>>({})

    render(
      <StateNamespaceProvider rootAtom={rootAtom} namespace={['test2']}>
        <TestCounter />
        <StateNamespaceProvider namespace={['parent']}>
          <TestCounter />
          <StateNamespaceProvider namespace={['child']}>
            <TestCounter />
          </StateNamespaceProvider>
        </StateNamespaceProvider>
      </StateNamespaceProvider>,
    )

    const rootButton = screen.getByTestId('counter-test2')
    const parentButton = screen.getByTestId('counter-test2-parent')
    const childButton = screen.getByTestId('counter-test2-parent-child')

    // Click nested counters
    fireEvent.click(rootButton)
    fireEvent.click(parentButton)
    fireEvent.click(childButton)

    expect(rootButton.textContent).toBe('Count: 1')
    expect(parentButton.textContent).toBe('Count: 1')
    expect(childButton.textContent).toBe('Count: 1')
  })

  it('preserves state updates within namespaces', () => {
    const rootAtom = atom<Record<string, unknown>>({})

    const { rerender } = render(
      <StateNamespaceProvider rootAtom={rootAtom}>
        <TestCounter namespace={['test']} />
      </StateNamespaceProvider>,
    )

    const button = screen.getByTestId('counter-test')
    fireEvent.click(button)
    expect(button.textContent).toBe('Count: 1')

    // Rerender and verify state persistence
    rerender(
      <StateNamespaceProvider rootAtom={rootAtom}>
        <TestCounter namespace={['test']} />
      </StateNamespaceProvider>,
    )

    expect(screen.getByTestId('counter-test').textContent).toBe('Count: 1')
  })

  describe('useStateNamespace', () => {
    it('combines paths with context correctly', () => {
      const rootAtom = atom<Record<string, unknown>>({})

      function TestComponent() {
        const { namespace: path } = useStateNamespace(['child', 'path'])
        return <div data-testid="path">{path.join('/')}</div>
      }

      render(
        <StateNamespaceProvider namespace={['parent']} rootAtom={rootAtom}>
          <TestComponent />
        </StateNamespaceProvider>,
      )

      expect(screen.getByTestId('path').textContent).toBe('parent/child/path')
    })

    it('handles empty context path', () => {
      function TestComponent() {
        const { namespace: path } = useStateNamespace(['test'])
        return <div data-testid="path">{path.join('/')}</div>
      }

      render(<TestComponent />)
      expect(screen.getByTestId('path').textContent).toBe('test')
    })

    it('handles empty input path', () => {
      function TestComponent() {
        const { namespace: path } = useStateNamespace()
        return <div data-testid="path">{path.join('/')}</div>
      }

      render(
        <StateNamespaceProvider namespace={['context']}>
          <TestComponent />
        </StateNamespaceProvider>,
      )
      expect(screen.getByTestId('path').textContent).toBe('context')
    })
  })

  it('handles custom namespace paths correctly', () => {
    const rootAtom = atom<Record<string, unknown>>({})

    function CustomNamespacedCounter() {
      const namespace = useStateNamespace(['custom', 'path'])
      const [count, setCount] = useStateAtom(namespace, 'count', 0)
      return (
        <button
          onClick={() => setCount((c: number) => c + 1)}
          data-testid="custom-counter"
        >
          Count: {count}
        </button>
      )
    }

    render(
      <StateNamespaceProvider rootAtom={rootAtom}>
        <CustomNamespacedCounter />
      </StateNamespaceProvider>,
    )

    const button = screen.getByTestId('custom-counter')
    fireEvent.click(button)
    expect(button.textContent).toBe('Count: 1')
  })

  it('works without StateNamespaceProvider using default namespace', () => {
    function DefaultCounter() {
      const [count, setCount] = useStateAtom(null, 'count', 0)
      return (
        <button
          onClick={() => setCount((c: number) => c + 1)}
          data-testid="default-counter"
        >
          Count: {count}
        </button>
      )
    }

    render(<DefaultCounter />)

    const button = screen.getByTestId('default-counter')

    // Initial state
    expect(button.textContent).toBe('Count: 0')

    // Update state
    fireEvent.click(button)
    expect(button.textContent).toBe('Count: 1')

    // Update again
    fireEvent.click(button)
    expect(button.textContent).toBe('Count: 2')
  })

  it('only rerenders when observed value changes', () => {
    const rootAtom = atom<Record<string, unknown>>({})
    let renderCount1 = 0
    let renderCount2 = 0

    function Counter1() {
      const [count, setCount] = useStateAtom(null, 'count1', 0)
      renderCount1++
      return (
        <button onClick={() => setCount((c) => c + 1)} data-testid="counter1">
          Count1: {count}
        </button>
      )
    }

    function Counter2() {
      const [count, setCount] = useStateAtom(null, 'count2', 0)
      renderCount2++
      return (
        <button onClick={() => setCount((c) => c + 1)} data-testid="counter2">
          Count2: {count}
        </button>
      )
    }

    render(
      <StateNamespaceProvider rootAtom={rootAtom}>
        <Counter1 />
        <Counter2 />
      </StateNamespaceProvider>,
    )

    const initialRenderCount1 = renderCount1
    const initialRenderCount2 = renderCount2
    expect(initialRenderCount1).toBeGreaterThanOrEqual(1)
    expect(initialRenderCount2).toBeGreaterThanOrEqual(1)

    // Update counter1
    fireEvent.click(screen.getByTestId('counter1'))
    expect(renderCount1).toBe(initialRenderCount1 + 1)
    expect(renderCount2).toBe(initialRenderCount2)

    // Update counter2
    fireEvent.click(screen.getByTestId('counter2'))
    expect(renderCount1).toBe(initialRenderCount1 + 1)
    expect(renderCount2).toBe(initialRenderCount2 + 1)
  })

  describe('StateNamespaceReducerAtom', () => {
    // Define a simple counter reducer for testing
    type CounterState = { value: number }
    type CounterAction = { type: 'INCREMENT' } | { type: 'DECREMENT' }

    const counterReducer = (
      state: CounterState,
      action: CounterAction,
    ): CounterState => {
      switch (action.type) {
        case 'INCREMENT':
          return { value: state.value + 1 }
        case 'DECREMENT':
          return { value: state.value - 1 }
        default:
          return state
      }
    }

    const initialState: CounterState = { value: 0 }

    function ReducerCounter({ namespace }: { namespace?: string[] }) {
      return (
        <StateNamespaceProvider namespace={namespace}>
          <ReducerCounterContent />
        </StateNamespaceProvider>
      )
    }

    function ReducerCounterContent() {
      const [state, dispatch] = useStateReducerAtom<
        CounterState,
        CounterAction
      >(null, 'reducerCount', counterReducer, initialState)

      const { namespace: contextNamespace } = useParentStateNamespace()
      const testId =
        contextNamespace.length > 0 ?
          `reducer-counter-${contextNamespace.join('-')}`
        : 'reducer-counter-root'

      return (
        <div>
          <div data-testid={testId}>Count: {state.value}</div>
          <button
            onClick={() => dispatch({ type: 'INCREMENT' })}
            data-testid={`${testId}-increment`}
          >
            Increment
          </button>
          <button
            onClick={() => dispatch({ type: 'DECREMENT' })}
            data-testid={`${testId}-decrement`}
          >
            Decrement
          </button>
        </div>
      )
    }

    it('handles basic reducer state updates', () => {
      const rootAtom = atom<Record<string, unknown>>({})

      const { getByTestId } = render(
        <StateNamespaceProvider rootAtom={rootAtom}>
          <ReducerCounter />
        </StateNamespaceProvider>,
      )

      const increment = getByTestId('reducer-counter-root-increment')
      const decrement = getByTestId('reducer-counter-root-decrement')
      const display = getByTestId('reducer-counter-root')

      // Initial state
      expect(display.textContent).toBe('Count: 0')

      // Increment twice
      fireEvent.click(increment)
      expect(display.textContent).toBe('Count: 1')

      fireEvent.click(increment)
      expect(display.textContent).toBe('Count: 2')

      // Decrement once
      fireEvent.click(decrement)
      expect(display.textContent).toBe('Count: 1')
    })

    it('maintains isolated reducer state for different namespaces', () => {
      const rootAtom = atom<Record<string, unknown>>({})

      render(
        <StateNamespaceProvider rootAtom={rootAtom} namespace={['test']}>
          <ReducerCounter />
          <ReducerCounter namespace={['ns1']} />
          <ReducerCounter namespace={['ns2']} />
        </StateNamespaceProvider>,
      )

      const rootIncrement = screen.getByTestId('reducer-counter-test-increment')
      const ns1Increment = screen.getByTestId(
        'reducer-counter-test-ns1-increment',
      )

      const rootDisplay = screen.getByTestId('reducer-counter-test')
      const ns1Display = screen.getByTestId('reducer-counter-test-ns1')
      const ns2Display = screen.getByTestId('reducer-counter-test-ns2')

      fireEvent.click(rootIncrement)
      expect(rootDisplay.textContent).toBe('Count: 1')
      expect(ns1Display.textContent).toBe('Count: 0')
      expect(ns2Display.textContent).toBe('Count: 0')

      fireEvent.click(ns1Increment)
      expect(rootDisplay.textContent).toBe('Count: 1')
      expect(ns1Display.textContent).toBe('Count: 1')
      expect(ns2Display.textContent).toBe('Count: 0')
    })

    it('persists reducer state across rerenders', () => {
      const rootAtom = atom<Record<string, unknown>>({})

      const { rerender } = render(
        <StateNamespaceProvider rootAtom={rootAtom}>
          <ReducerCounter namespace={['persist']} />
        </StateNamespaceProvider>,
      )

      const increment = screen.getByTestId('reducer-counter-persist-increment')
      const display = screen.getByTestId('reducer-counter-persist')

      fireEvent.click(increment)
      expect(display.textContent).toBe('Count: 1')

      rerender(
        <StateNamespaceProvider rootAtom={rootAtom}>
          <ReducerCounter namespace={['persist']} />
        </StateNamespaceProvider>,
      )

      expect(screen.getByTestId('reducer-counter-persist').textContent).toBe(
        'Count: 1',
      )
    })
  })

  it('inherits path from stateNamespace prop', () => {
    const rootAtom = atom<Record<string, unknown>>({})
    const testNamespace = {
      namespace: ['inherited', 'path'],
      stateAtom: rootAtom,
    }

    render(
      <StateNamespaceProvider
        rootAtom={rootAtom}
        stateNamespace={testNamespace}
      >
        <CounterContent />
      </StateNamespaceProvider>,
    )

    const button = screen.getByTestId('counter-inherited-path')

    // Initial state
    expect(button.textContent).toBe('Count: 0')

    // Update state
    fireEvent.click(button)
    expect(button.textContent).toBe('Count: 1')
  })

  it('does not prepend the parent path when stateNamespace is already expanded', () => {
    const rootAtom = atom<Record<string, unknown>>({})

    function NestedNamespaceComponent() {
      const stateNamespace = useStateNamespace(['session'])
      return (
        <StateNamespaceProvider stateNamespace={stateNamespace}>
          <CounterContent />
        </StateNamespaceProvider>
      )
    }

    render(
      <StateNamespaceProvider
        rootAtom={rootAtom}
        namespace={['shell-tab', 'home']}
      >
        <NestedNamespaceComponent />
      </StateNamespaceProvider>,
    )

    expect(
      screen.getByTestId('counter-shell-tab-home-session').textContent,
    ).toBe('Count: 0')
    expect(
      screen.queryByTestId('counter-shell-tab-home-shell-tab-home-session'),
    ).toBeNull()
  })

  it('handles nullable string state correctly', () => {
    const rootAtom = atom<Record<string, unknown>>({})

    function NullableStringComponent() {
      const [value, setValue] = useStateAtom<string | null>(
        null,
        'nullableStr',
        null,
      )
      return (
        <div>
          <div data-testid="display">Value: {value ?? 'null'}</div>
          <button onClick={() => setValue('test')} data-testid="set-value">
            Set Value
          </button>
          <button onClick={() => setValue(null)} data-testid="set-null">
            Set Null
          </button>
        </div>
      )
    }

    render(
      <StateNamespaceProvider rootAtom={rootAtom}>
        <NullableStringComponent />
      </StateNamespaceProvider>,
    )

    const display = screen.getByTestId('display')
    const setValue = screen.getByTestId('set-value')
    const setNull = screen.getByTestId('set-null')

    // Initial null state
    expect(display.textContent).toBe('Value: null')

    // Set to string value
    fireEvent.click(setValue)
    expect(display.textContent).toBe('Value: test')

    // Set back to null
    fireEvent.click(setNull)
    expect(display.textContent).toBe('Value: null')
  })

  it('removes keys when value equals default and cleans up empty objects', () => {
    const rootAtom = atom<Record<string, unknown>>({})

    function CleanupTestComponent() {
      const [value1, setValue1] = useStateAtom(null, 'test1', 0)
      const [value2, setValue2] = useStateAtom(
        useStateNamespace(['nested']),
        'test2',
        'default',
      )
      return (
        <div>
          <div data-testid="value1">Value1: {value1}</div>
          <div data-testid="value2">Value2: {value2}</div>
          <button onClick={() => setValue1(0)} data-testid="reset1">
            Reset Value1
          </button>
          <button onClick={() => setValue2('default')} data-testid="reset2">
            Reset Value2
          </button>
          <button onClick={() => setValue1(1)} data-testid="change1">
            Change Value1
          </button>
          <pre data-testid="state">
            {JSON.stringify(rootAtom.get(), null, 2)}
          </pre>
        </div>
      )
    }

    render(
      <StateNamespaceProvider rootAtom={rootAtom}>
        <CleanupTestComponent />
      </StateNamespaceProvider>,
    )

    const change1 = screen.getByTestId('change1')
    const reset1 = screen.getByTestId('reset1')
    const reset2 = screen.getByTestId('reset2')
    const state = screen.getByTestId('state')

    // Initial state should be empty
    expect(JSON.parse(state.textContent || '{}')).toEqual({})

    // Change value1 to non-default
    fireEvent.click(change1)
    expect(JSON.parse(state.textContent || '{}')).toEqual({
      test1: 1,
    })

    // Reset value1 to default - key should be removed
    fireEvent.click(reset1)
    expect(JSON.parse(state.textContent || '{}')).toEqual({})

    // Reset value2 when it's already default - nested object should be removed
    fireEvent.click(reset2)
    expect(JSON.parse(state.textContent || '{}')).toEqual({})
  })
})
