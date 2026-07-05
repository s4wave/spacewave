import { lazy, Suspense, type ComponentType, type ReactNode } from 'react'

import { Routes, Route } from '@s4wave/web/router/router.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'

function createRouteGroup(routes: ReactNode): ComponentType {
  return function LazyRouteGroup() {
    return <Routes fullPath>{routes}</Routes>
  }
}

const LazyLandingRoutes = lazy(async () => {
  const { LandingRoutes } = await import('./LandingRoutes.js')
  return { default: createRouteGroup(LandingRoutes) }
})

const LazyDocsRoutes = lazy(async () => {
  const { DocsRoutes } = await import('./DocsRoutes.js')
  return { default: createRouteGroup(DocsRoutes) }
})

const LazyBlogRoutes = lazy(async () => {
  const { BlogRoutes } = await import('./BlogRoutes.js')
  return { default: createRouteGroup(BlogRoutes) }
})

const LazyAuthRoutes = lazy(async () => {
  const { AuthRoutes } = await import('./AuthRoutes.js')
  return { default: createRouteGroup(AuthRoutes) }
})

const LazySessionRoutes = lazy(async () => {
  const { SessionRoutes } = await import('./SessionRoutes.js')
  return { default: createRouteGroup(SessionRoutes) }
})

const LazyDisplayRoutes = lazy(async () => {
  const { DisplayRoutes } = await import('../display/DisplayRoutes.js')
  return { default: createRouteGroup(DisplayRoutes) }
})

const LazyAppSession = lazy(async () => {
  const { AppSession } = await import('../AppSession.js')
  return { default: AppSession }
})

const LazyDebugRoutes = lazy(async () => {
  const { DebugRoutes } = await import('./DebugRoutes.js')
  return { default: createRouteGroup(DebugRoutes) }
})

function LazyRoute(props: { component: ComponentType }) {
  const Component = props.component
  return (
    <Suspense fallback={null}>
      <Component />
    </Suspense>
  )
}

// AppRoutes renders the appropriate content based on the current path.
export function AppRoutes() {
  return (
    <Routes fullPath>
      <Route path="/">
        <LazyRoute component={LazyLandingRoutes} />
      </Route>
      <Route path="/landing/*">
        <LazyRoute component={LazyLandingRoutes} />
      </Route>
      <Route path="/community">
        <LazyRoute component={LazyLandingRoutes} />
      </Route>
      <Route path="/tos">
        <LazyRoute component={LazyLandingRoutes} />
      </Route>
      <Route path="/privacy">
        <LazyRoute component={LazyLandingRoutes} />
      </Route>
      <Route path="/pricing">
        <LazyRoute component={LazyLandingRoutes} />
      </Route>
      <Route path="/dmca">
        <LazyRoute component={LazyLandingRoutes} />
      </Route>
      <Route path="/licenses">
        <LazyRoute component={LazyLandingRoutes} />
      </Route>
      <Route path="/changelog">
        <LazyRoute component={LazyLandingRoutes} />
      </Route>
      <Route path="/download/*">
        <LazyRoute component={LazyLandingRoutes} />
      </Route>
      <Route path="/docs/*">
        <LazyRoute component={LazyDocsRoutes} />
      </Route>
      <Route path="/blog/*">
        <LazyRoute component={LazyBlogRoutes} />
      </Route>
      <Route path="/sessions">
        <LazyRoute component={LazyAuthRoutes} />
      </Route>
      <Route path="/auth/*">
        <LazyRoute component={LazyAuthRoutes} />
      </Route>
      <Route path="/login">
        <LazyRoute component={LazyAuthRoutes} />
      </Route>
      <Route path="/signup">
        <LazyRoute component={LazyAuthRoutes} />
      </Route>
      <Route path="/recover">
        <LazyRoute component={LazyAuthRoutes} />
      </Route>
      <Route path="/checkout/*">
        <LazyRoute component={LazySessionRoutes} />
      </Route>
      <Route path="/join/*">
        <LazyRoute component={LazySessionRoutes} />
      </Route>
      <Route path="/pair/*">
        <LazyRoute component={LazySessionRoutes} />
      </Route>
      <Route path="/display/*">
        <LazyRoute component={LazyDisplayRoutes} />
      </Route>
      <Route path="/quickstart/*">
        <LazyRoute component={LazySessionRoutes} />
      </Route>
      <Route path="/u/:sessionIndex/*">
        <LazyRoute component={LazyAppSession} />
      </Route>
      <Route path="/debug/*">
        <LazyRoute component={LazyDebugRoutes} />
      </Route>
      <Route path="*">
        <NavigatePath to="/" replace />
      </Route>
    </Routes>
  )
}
