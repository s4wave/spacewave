// Lightweight hydration entry for prerendered static pages.
// Imports React and the page components, calls hydrateRoot() to attach
// event handlers (accordions, scroll, navigation). Does NOT load the
// bldr runtime, SharedWorker, or WASM - those load later via the
// deferred entrypoint boot (__swDeferBoot / __swBoot).

import { hydrateRoot, type Root } from 'react-dom/client'

declare global {
  var __swEntry: string | undefined
  var __swGenerationId: string | undefined
  var __swReady: Promise<void> | undefined
  var __swBoot: ((hash: string) => void) | undefined
  var __swPrerenderRoot: Root | undefined
  var __swPrerenderContainer: HTMLElement | undefined
}

import { StaticProvider } from './StaticContext.js'
import { getStaticPageComponent } from './static-pages.js'
import { isStaticRoute } from '@s4wave/web/router/static-routes.js'
import { markInteracted } from '@s4wave/web/state/interaction.js'
import { RouterProvider, type To } from '@s4wave/web/router/router.js'
import { Landing } from '@s4wave/app/landing/Landing.js'
import { BlogPostPage } from '@s4wave/app/blog/BlogPost.js'
import { BlogIndex } from '@s4wave/app/blog/BlogIndex.js'
import { BlogTagPage } from '@s4wave/app/blog/BlogTagPage.js'
import type { BlogPost } from '@s4wave/app/blog/types.js'

// awaitBoot waits for __swReady then calls __swBoot with the given path.
function awaitBoot(path: string) {
  const ready = globalThis.__swReady
  if (ready) {
    void ready.then(() => globalThis.__swBoot?.(path))
  }
}

// handleNavigate routes navigation from hydrated static pages.
// Static page targets do a full page load (they are pre-rendered).
// App route targets boot the full app via __swBoot.
function handleNavigate(to: To) {
  const path = typeof to === 'string' ? to : to.path
  if (!path) return

  if (isStaticRoute(path)) {
    window.location.href = path
    return
  }

  // App route: mark as interacted, then boot via __swBoot or import.
  markInteracted()

  if (globalThis.__swBoot) {
    globalThis.__swBoot(path)
    return
  }

  // __swBoot not yet available: show loading, await __swReady, then boot.
  const landing = document.getElementById('sw-landing')
  const loading = document.getElementById('sw-loading')
  if (landing) landing.style.display = 'none'
  if (loading) loading.style.display = ''

  awaitBoot(path)
  if (!globalThis.__swReady) {
    // Fallback: import entrypoint directly (no deferred boot).
    const entry = globalThis.__swEntry
    if (entry) {
      window.location.hash = path
      void import(/* @vite-ignore */ entry)
    }
  }
}

function readBlogData(): Record<string, unknown> | null {
  const el = document.getElementById('blog-data')
  if (!el?.textContent) return null
  return JSON.parse(el.textContent) as Record<string, unknown>
}

const pathname = window.location.pathname

// Auto-boot for return visitors on root path.
// If hasSession is set, the bootstrap script already showed the loading
// screen. hydrate.tsx awaits __swReady and calls __swBoot to render the app.
if (pathname === '/' && window.location.hash.length > 1) {
  awaitBoot(window.location.hash)
} else if (pathname === '/' && localStorage.getItem('spacewave-has-session')) {
  awaitBoot('')
} else if (pathname === '/') {
  // New visitor on root: hydrate the landing page inside sw-landing.
  const container = document.getElementById('sw-landing')
  if (container) {
    globalThis.__swPrerenderContainer = container
    globalThis.__swPrerenderRoot = hydrateRoot(
      container,
      <RouterProvider path="/" onNavigate={handleNavigate}>
        <StaticProvider>
          <Landing />
        </StaticProvider>
      </RouterProvider>,
    )
  }
} else if (pathname.startsWith('/blog')) {
  // Blog pages: read serialized data from blog-data script tag.
  const container = document.getElementById('bldr-root')
  if (container?.hasAttribute('data-prerendered')) {
    const data = readBlogData()
    if (data) {
      let element: React.ReactElement | null = null

      if (data.type === 'post') {
        const post: BlogPost = {
          slug: data.slug as string,
          url: data.url as string,
          title: data.title as string,
          date: data.date as string,
          author: data.author as BlogPost['author'],
          authorSlug: data.authorSlug as string,
          summary: data.summary as string,
          tags: data.tags as string[],
          draft: data.draft as boolean,
          body: '',
        }
        element = (
          <BlogPostPage
            post={post}
            bodyHtml={data.bodyHtml as string}
            prevPost={data.prev as { title: string; url: string } | undefined}
            nextPost={data.next as { title: string; url: string } | undefined}
          />
        )
      } else if (data.type === 'index') {
        element = <BlogIndex posts={data.posts as BlogPost[]} />
      } else if (data.type === 'tag') {
        element = (
          <BlogTagPage
            tag={data.tag as string}
            posts={data.posts as BlogPost[]}
          />
        )
      }

      if (element) {
        globalThis.__swPrerenderContainer = container
        globalThis.__swPrerenderRoot = hydrateRoot(
          container,
          <RouterProvider path={pathname} onNavigate={handleNavigate}>
            <StaticProvider>{element}</StaticProvider>
          </RouterProvider>,
        )
      }
    }
  }
} else if (pathname.startsWith('/quickstart/')) {
  // Quickstart loading pages: hydrate, then auto-boot when WASM ready.
  const container = document.getElementById('bldr-root')
  if (container?.hasAttribute('data-prerendered')) {
    const Component = getStaticPageComponent(pathname)
    if (Component) {
      globalThis.__swPrerenderContainer = container
      globalThis.__swPrerenderRoot = hydrateRoot(
        container,
        <RouterProvider path={pathname} onNavigate={handleNavigate}>
          <StaticProvider>
            <Component />
          </StaticProvider>
        </RouterProvider>,
      )
      // Auto-transition to app quickstart when entrypoint is ready.
      awaitBoot('#' + pathname)
    }
  }
} else {
  // Other static pages: the server renders the component directly
  // inside bldr-root. Hydrate the full container.
  const container = document.getElementById('bldr-root')
  if (container?.hasAttribute('data-prerendered')) {
    const Component = getStaticPageComponent(pathname)
    if (Component) {
      globalThis.__swPrerenderContainer = container
      globalThis.__swPrerenderRoot = hydrateRoot(
        container,
        <RouterProvider path={pathname} onNavigate={handleNavigate}>
          <StaticProvider>
            <Component />
          </StaticProvider>
        </RouterProvider>,
      )
    }
  }
}
