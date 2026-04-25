import React, { useEffect } from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, renderHook, cleanup } from '@testing-library/react'
import {
  Router,
  Route,
  Routes,
  useRouter,
  useParams,
  usePath,
  useNavigate,
  resolvePath,
} from './router.js'

describe('Router', () => {
  describe('resolvePath', () => {
    afterEach(() => {
      cleanup()
    })

    it('handles absolute paths', () => {
      expect(resolvePath('/current', { path: '/absolute' })).toBe('/absolute')
    })

    it('handles relative paths', () => {
      expect(resolvePath('/current/path', { path: '../other' })).toBe(
        '/current/other',
      )
      expect(resolvePath('/current/path', { path: './other' })).toBe(
        '/current/path/other',
      )
    })

    it('handles multiple parent directory traversals', () => {
      expect(resolvePath('/a/b/c', { path: '../../d' })).toBe('/a/d')
      expect(resolvePath('/a/b/c', { path: '../../../d' })).toBe('/d')
      expect(resolvePath('/a/b/c', { path: '../../../../d' })).toBe('/d')
    })

    it('handles absolute paths with parent traversals', () => {
      expect(resolvePath('/current', { path: '/../other' })).toBe('/other')
      expect(resolvePath('/current', { path: '/../../other' })).toBe('/other')
      expect(resolvePath('/a/b', { path: '/c/../d' })).toBe('/d')
    })

    it('handles edge cases', () => {
      expect(resolvePath('/current', { path: '' })).toBe('/current')
      expect(resolvePath('/current', { path: '.' })).toBe('/current')
      expect(resolvePath('/current', { path: '..' })).toBe('/')
      expect(resolvePath('/current/path', { path: '../.' })).toBe('/current')
      expect(resolvePath('/current/path', { path: './..' })).toBe('/current')
    })

    it('handles mixed forward and parent navigation', () => {
      expect(resolvePath('/a/b', { path: '../c/./d/../e' })).toBe('/a/c/e')
      expect(resolvePath('/a/b', { path: 'c/../../d' })).toBe('/a/d')
      expect(resolvePath('/a/b', { path: './c/../d/./e' })).toBe('/a/b/d/e')
    })

    it('maintains parent paths through nested routes', () => {
      const TestComponent = () => {
        const { parentPaths, params } = useRouter()
        return (
          <div>
            <div data-testid="parent-paths">{JSON.stringify(parentPaths)}</div>
            <div data-testid="wildcard">{params['*']}</div>
          </div>
        )
      }

      const { getByTestId } = render(
        <Router
          path="/u/2/so/test-space/k/object-layout/main"
          onNavigate={vi.fn()}
        >
          <Routes>
            <Route path="/u/2/*">
              <Routes>
                <Route path="so/test-space/*">
                  <TestComponent />
                </Route>
              </Routes>
            </Route>
          </Routes>
        </Router>,
      )

      expect(JSON.parse(getByTestId('parent-paths').textContent ?? '')).toEqual(
        ['/u/2', 'so/test-space'],
      )
      expect(getByTestId('wildcard').textContent).toBe('k/object-layout/main')
    })

    it('does not add to parent paths for non-wildcard routes', () => {
      const TestComponent = () => {
        const { parentPaths } = useRouter()
        return (
          <div data-testid="parent-paths">{JSON.stringify(parentPaths)}</div>
        )
      }

      const { getByTestId } = render(
        <Router path="/static/path" onNavigate={vi.fn()}>
          <Routes>
            <Route path="/static/path">
              <TestComponent />
            </Route>
          </Routes>
        </Router>,
      )

      expect(JSON.parse(getByTestId('parent-paths').textContent ?? '')).toEqual(
        [],
      )
    })
  })

  describe('Router Component', () => {
    const defaultPath = '/test'
    const onNavigate = vi.fn()

    beforeEach(() => {
      onNavigate.mockClear()
    })

    it('renders children', () => {
      const { getByText } = render(
        <Router path={defaultPath} onNavigate={onNavigate}>
          <div>Test Content</div>
        </Router>,
      )
      expect(getByText('Test Content')).toBeTruthy()
    })

    it('provides router context', () => {
      const TestComponent = () => {
        const router = useRouter()
        return <div>Path: {router.path}</div>
      }

      const { getByText } = render(
        <Router path={defaultPath} onNavigate={onNavigate}>
          <TestComponent />
        </Router>,
      )
      expect(getByText('Path: /test')).toBeTruthy()
    })
  })

  describe('Routes and Route', () => {
    const onNavigate = vi.fn()

    beforeEach(() => {
      onNavigate.mockClear()
    })

    it('renders matching route', () => {
      const { container } = render(
        <Router path="/users/123" onNavigate={onNavigate}>
          <Routes>
            <Route path="/users/:id">
              <div data-testid="user-profile">User Profile</div>
            </Route>
            <Route path="/about">
              <div>About</div>
            </Route>
          </Routes>
        </Router>,
      )
      expect(
        container.querySelector('[data-testid="user-profile"]'),
      ).toBeTruthy()
    })

    it('handles route parameters', () => {
      const TestComponent = () => {
        const params = useParams()
        return <div>User ID: {params.id}</div>
      }

      const { getByText } = render(
        <Router path="/users/123" onNavigate={onNavigate}>
          <Routes>
            <Route path="/users/:id">
              <TestComponent />
            </Route>
          </Routes>
        </Router>,
      )
      expect(getByText('User ID: 123')).toBeTruthy()
    })

    it('decodes encoded route parameters', () => {
      const TestComponent = () => {
        const params = useParams()
        return <div>User ID: {params.id}</div>
      }

      const { getByText } = render(
        <Router path="/users/video%20with%20spaces.mp4" onNavigate={onNavigate}>
          <Routes>
            <Route path="/users/:id">
              <TestComponent />
            </Route>
          </Routes>
        </Router>,
      )
      expect(getByText('User ID: video with spaces.mp4')).toBeTruthy()
    })

    it('preserves literal percent characters in decoded route parameters', () => {
      const TestComponent = () => {
        const params = useParams()
        return <div>User ID: {params.id}</div>
      }

      const { getByText } = render(
        <Router path="/users/100% legit.txt" onNavigate={onNavigate}>
          <Routes>
            <Route path="/users/:id">
              <TestComponent />
            </Route>
          </Routes>
        </Router>,
      )
      expect(getByText('User ID: 100% legit.txt')).toBeTruthy()
    })

    it('handles wildcard routes', () => {
      const TestComponent = () => {
        const params = useParams()
        return <div>Wildcard: {params['*']}</div>
      }

      const { getByText } = render(
        <Router path="/catch/all/path" onNavigate={onNavigate}>
          <Routes>
            <Route path="/catch/*">
              <TestComponent />
            </Route>
          </Routes>
        </Router>,
      )
      expect(getByText('Wildcard: all/path')).toBeTruthy()
    })

    it('decodes encoded wildcard paths', () => {
      const TestComponent = () => {
        const params = useParams()
        return <div>Wildcard: {params['*']}</div>
      }

      const { getByText } = render(
        <Router
          path="/so/files/-/test/dir/video%20with%20spaces.mp4"
          onNavigate={onNavigate}
        >
          <Routes>
            <Route path="/so/*">
              <TestComponent />
            </Route>
          </Routes>
        </Router>,
      )
      expect(
        getByText('Wildcard: files/-/test/dir/video with spaces.mp4'),
      ).toBeTruthy()
    })

    it('uses provided path prop instead of router context path', () => {
      const { getByText } = render(
        <Router path="/users/123" onNavigate={onNavigate}>
          <Routes path="/about">
            <Route path="/about">
              <div>About Page</div>
            </Route>
            <Route path="/users/:id">
              <div>User Profile</div>
            </Route>
          </Routes>
        </Router>,
      )
      expect(getByText('About Page')).toBeTruthy()
    })

    it('falls back to router context path when path prop is not provided', () => {
      const { container } = render(
        <Router path="/users/123" onNavigate={onNavigate}>
          <Routes>
            <Route path="/about">
              <div>About Page</div>
            </Route>
            <Route path="/users/:id">
              <div data-testid="user-profile">User Profile</div>
            </Route>
          </Routes>
        </Router>,
      )
      expect(
        container.querySelector('[data-testid="user-profile"]'),
      ).toBeTruthy()
    })

    it('uses wildcard match by default in nested routes', () => {
      const { container } = render(
        <Router path="/app/dashboard/settings" onNavigate={onNavigate}>
          <Routes>
            <Route path="/app/*">
              <Routes>
                <Route path="/dashboard/settings">
                  <div data-testid="settings">Settings</div>
                </Route>
              </Routes>
            </Route>
          </Routes>
        </Router>,
      )
      expect(container.querySelector('[data-testid="settings"]')).toBeTruthy()
    })

    it('uses full path when fullPath prop is true', () => {
      const { container } = render(
        <Router path="/app/dashboard/settings" onNavigate={onNavigate}>
          <Routes>
            <Route path="/app/*">
              <Routes fullPath>
                <Route path="/app/dashboard/settings">
                  <div data-testid="settings">Settings</div>
                </Route>
              </Routes>
            </Route>
          </Routes>
        </Router>,
      )
      expect(container.querySelector('[data-testid="settings"]')).toBeTruthy()
    })

    it('should not re-run parent effects when only child routes change', () => {
      let effectRuns = 0

      // Parent dashboard component that contains the effect we want to monitor
      const Dashboard = () => {
        useEffect(() => {
          effectRuns++
          return () => {}
        }, [])

        return (
          <div>
            <h1>Dashboard</h1>
            <Routes>
              <Route path="/settings">
                <div data-testid="settings">Settings Page</div>
              </Route>
              <Route path="/about">
                <div data-testid="about">About Page</div>
              </Route>
            </Routes>
          </div>
        )
      }

      // Root component with main routing
      const Root = () => (
        <Routes>
          <Route path="/dashboard/*">
            <Dashboard />
          </Route>
        </Routes>
      )

      const { container, rerender } = render(
        <Router path="/dashboard/settings" onNavigate={onNavigate}>
          <Root />
        </Router>,
      )

      // Verify initial render
      expect(container.querySelector('[data-testid="settings"]')).toBeTruthy()
      expect(effectRuns).toBe(1)

      // Navigate to about page
      rerender(
        <Router path="/dashboard/about" onNavigate={onNavigate}>
          <Root />
        </Router>,
      )

      // Verify about page is shown
      expect(container.querySelector('[data-testid="about"]')).toBeTruthy()

      // Effect should not have run again
      expect(effectRuns).toBe(1)
    })
  })

  describe('Hooks', () => {
    const defaultPath = '/test'
    const onNavigate = vi.fn()

    beforeEach(() => {
      onNavigate.mockClear()
    })

    it('useNavigate triggers navigation', () => {
      const TestComponent = () => {
        const navigate = useNavigate()
        useEffect(() => {
          navigate({ path: '/new-path' })
        }, [navigate])
        return null
      }

      render(
        <Router path={defaultPath} onNavigate={onNavigate}>
          <TestComponent />
        </Router>,
      )

      expect(onNavigate).toHaveBeenCalledWith({ path: '/new-path' })
    })

    it('usePath returns current path', () => {
      const { result } = renderHook(() => usePath(), {
        wrapper: ({ children }) => (
          <Router path={defaultPath} onNavigate={onNavigate}>
            {children}
          </Router>
        ),
      })

      expect(result.current).toBe(defaultPath)
    })

    it('useParams returns empty object when no params match', () => {
      const { result } = renderHook(() => useParams(), {
        wrapper: ({ children }) => (
          <Router path={defaultPath} onNavigate={onNavigate}>
            {children}
          </Router>
        ),
      })

      expect(result.current).toEqual({})
    })
  })
})
