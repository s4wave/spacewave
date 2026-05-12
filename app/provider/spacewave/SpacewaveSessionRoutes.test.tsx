import { isValidElement, type ReactElement, type ReactNode } from 'react'
import { describe, expect, it } from 'vitest'

import { SessionFlowFrame } from '@s4wave/app/session/SessionFlowFrame.js'

import { spacewaveSessionRoutes } from './SpacewaveSessionRoutes.js'

describe('spacewaveSessionRoutes', () => {
  it('keeps plan subflows inside the session frame', () => {
    const routes = spacewaveSessionRoutes(undefined, { fallbackPath: '/u/7' })
    const planRoute = findRoute(routes, '/plan')
    const planFreeRoute = findRoute(routes, '/plan/free')

    expectFrame(planRoute)
    expectFrame(planFreeRoute)
  })
})

interface RouteLikeProps {
  path: string
  children: ReactNode
}

interface FrameLikeProps {
  fallbackPath: string
}

function findRoute(
  routes: ReactNode[],
  path: string,
): ReactElement<RouteLikeProps> {
  const route = routes.find(
    (candidate): candidate is ReactElement<RouteLikeProps> =>
      isValidElement<RouteLikeProps>(candidate) &&
      candidate.props.path === path,
  )
  if (!route) throw new Error(`missing route ${path}`)
  return route
}

function expectFrame(route: ReactElement<RouteLikeProps>) {
  const child = route.props.children
  if (!isValidElement<FrameLikeProps>(child)) {
    throw new Error(`route ${route.props.path} is not framed`)
  }
  expect(child.type).toBe(SessionFlowFrame)
  expect(child.props.fallbackPath).toBe('/u/7')
}
