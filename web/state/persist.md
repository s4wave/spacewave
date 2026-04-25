# UI Component State Persistence Design

## Background

We need a state management solution that allows components to operate
independently while enabling centralized persistence of their states. The system
must handle dynamic component hierarchies where components can be added,
removed, or nested at runtime.

## Goals

1. Persist UI component states across sessions
2. Keep component implementations simple and decoupled from persistence logic
3. Support dynamic, nested component hierarchies
4. Maintain type safety and developer ergonomics

## Requirements

- Components should be able to define and manage their state without knowledge
  of persistence
- State persistence should be opt-in via props/context
- Support for nested state namespacing
- Single source of truth for persisted state
- Efficient updates that only affect changed components
- Type-safe state management
- Simple debugging and state inspection

## Implementation Details

### 1. Basic Atom Implementation

The foundation is a simple Atom class that:
- Holds a value
- Maintains a list of subscribers
- Notifies subscribers when the value changes

```typescript
class Atom<T> {
  private value: T
  private listeners: Set<() => void>

  constructor(initialValue: T) {
    this.value = initialValue
    this.listeners = new Set()
  }

  get(): T {
    return this.value
  }

  set(newValue: T) {
    this.value = newValue
    this.notify()
  }

  subscribe(callback: () => void): () => void {
    this.listeners.add(callback)
    return () => this.listeners.delete(callback)
  }

  private notify() {
    this.listeners.forEach(listener => listener())
  }
}
```

### 2. Core Components

#### StateNamespaceProvider
Provides namespace context for nested state management:
- Manages hierarchical state structure
- Tracks current namespace path
- Provides access to root state atom
- Enables nested state management
- Accepts optional namespace array or StateNamespace for inheritance

#### State Management Hooks
- `useStateNamespace`: Creates namespace paths by combining context with additional segments
- `useStateAtom`: Manages state within a namespace with automatic persistence
- `useStateReducerAtom`: Provides reducer-style state management
- `useParentStateNamespace`: Retrieves parent namespace context

#### Persistence Layer
- Uses `atomWithLocalStorage` to create root atom with localStorage persistence
- Leverages SuperJSON for type serialization
- Handles automatic serialization/deserialization
- Provides synchronous storage interface

### Usage Example

```typescript
// Create a persisted root atom
const rootAtom = new Atom<Record<string, unknown>>({})

// Basic counter component
function Counter() {
  const [count, setCount] = useStateAtom(null, "count", 0)
  return (
    <button onClick={() => setCount(c => c + 1)}>
      Count: {count}
    </button>
  )
}

// Example with namespacing
function NamespacedCounter() {
  const namespace = useStateNamespace(["custom", "path"])
  const [count, setCount] = useStateAtom(namespace, "count", 0)
  return (
    <button onClick={() => setCount(c => c + 1)}>
      Namespaced Count: {count}
    </button>
  )
}

// App with nested state management
function App() {
  return (
    <StateNamespaceProvider rootAtom={rootAtom}>
      <Counter />
      <StateNamespaceProvider namespace="nested">
        <Counter />
        <StateDebugger />
      </StateNamespaceProvider>
      <NamespacedCounter />
    </StateNamespaceProvider>
  )
}
```

This creates a state structure like:
```json
{
  "count": 1,
  "nested": {
    "count": 2
  },
  "custom": {
    "path": {
      "count": 3
    }
  }
}
```
