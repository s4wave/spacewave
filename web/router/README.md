# Router

A lightweight, flexible router for React applications with support for nested routes, path parameters, and relative navigation.

## Basic Usage

```tsx
import { Router, Routes, Route } from './router'

function App() {
  return (
    <Router path="/dashboard" onNavigate={({ path }) => window.history.pushState(null, '', path)}>
      <Routes>
        <Route path="/dashboard">
          <DashboardPage />
        </Route>
        <Route path="/settings">
          <SettingsPage />
        </Route>
      </Routes>
    </Router>
  )
}
```

## Features

### Path Parameters

```tsx
// URL: /users/123
<Route path="/users/:id">
  <UserProfile />
</Route>

function UserProfile() {
  const { id } = useParams()
  // id = "123"
}
```

### Nested Routes

```tsx
<Routes>
  <Route path="/app/*">
    <Routes>
      <Route path="/dashboard">
        <Dashboard />
      </Route>
      <Route path="/settings">
        <Settings />
      </Route>
    </Routes>
  </Route>
</Routes>
```

### Navigation

```tsx
function NavigationExample() {
  const navigate = useNavigate()
  
  return (
    <button onClick={() => navigate({ path: '/dashboard' })}>
      Go to Dashboard
    </button>
  )
}
```

### Relative Paths

```tsx
// Current path: /users/list
navigate({ path: '../settings' }) // -> /users/settings
navigate({ path: './123' }) // -> /users/list/123
navigate({ path: '/dashboard' }) // -> /dashboard (absolute)
```

## API Reference

- `Router`: Main component that provides routing context
- `Routes`: Container for Route components
- `Route`: Defines a route pattern and its content

- `useNavigate()`: Hook for programmatic navigation
- `useParams()`: Hook to access route parameters
- `usePath()`: Hook to get current path
- `useRouter()`: Hook to access full router context

## Best Practices

- Use absolute paths for top-level routes
- Use relative paths for nested navigation
- Place more specific routes before wildcards
- Use the wildcard pattern (`*`) for nested routing
