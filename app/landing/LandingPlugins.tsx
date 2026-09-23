import { LuDatabase, LuLayoutGrid, LuPuzzle, LuTable } from 'react-icons/lu'

import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'

import { UseCaseDemo } from './use-case/UseCaseDemo.js'
import { UseCasePage } from './use-case/UseCasePage.js'

// PLUGINS_DEMO_QUERY is the query the demo suggests running.
const PLUGINS_DEMO_QUERY = 'SELECT name, role FROM quickstart.people'

export const metadata = {
  title: 'Spacewave Plugins - New kinds of objects, in your Space.',
  description:
    'Try the Spacewave SQL Database plugin in your browser. Plugins in Go or TypeScript add object types and viewers that run on your own devices.',
  canonicalPath: '/landing/plugins',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

// LandingPlugins renders the Plugins use-case page. live mounts the demo SQL
// Database, which a plugin provides.
export function LandingPlugins({ live = false }: { live?: boolean }) {
  const landingHref = useStaticHref('/landing')

  return (
    <UseCasePage
      icon={<LuPuzzle className="size-8" />}
      title="New kinds of objects, in your Space."
      subtitle="Plugins teach Spacewave new object types. The SQL Database below is a plugin: it loads when the Space needs it and runs on your device, not on a server."
      points={[
        {
          title: 'Go or TypeScript',
          body: 'The SQL Database plugin is written in Go and the Notebook in TypeScript. Both use the same SDK to register object types, viewers and quickstarts.',
        },
        {
          title: 'Loaded on demand',
          body: 'A Space loads the plugins its objects need. Each plugin runs in its own worker beside the app.',
        },
        {
          title: 'Data stays in the Space',
          body: 'Plugin objects are stored in the Space like files and notes, so they sync and back up the same way.',
        },
      ]}
      keepTitle="Keep your database"
      keepBody="The demo lives in memory and is gone when you leave. The SQL Database Quickstart creates the same Space in your browser's storage, where it stays until you delete it."
      primary={{
        href: '#/quickstart/sql',
        label: 'Create a SQL Database',
        icon: LuDatabase,
      }}
      secondary={{
        href: landingHref,
        label: 'See all features',
        icon: LuLayoutGrid,
      }}
    >
      <UseCaseDemo
        demo="plugins"
        live={live}
        label="SQL Database"
        poster={<PluginsPoster />}
        suggestions={[
          'Expand the quickstart schema to list its tables.',
          `Open Query Editor and run ${PLUGINS_DEMO_QUERY}.`,
          'Change the query, run it again, then press Reset to start over.',
        ]}
      />
    </UseCasePage>
  )
}

// PluginsPoster previews the SQL Database Quickstart: its schemas and the
// result of the suggested query.
function PluginsPoster() {
  const rows = [
    { name: 'ada', role: 'analyst' },
    { name: 'grace', role: 'engineer' },
  ]

  return (
    <div className="flex h-full text-sm">
      <div className="border-foreground/10 flex w-48 shrink-0 flex-col gap-1 border-r p-3">
        <span className="text-foreground-alt text-metadata px-2 pb-1 font-semibold tracking-wide uppercase">
          My SQL Database
        </span>
        <span className="text-foreground flex items-center gap-2 px-2 py-1">
          <LuDatabase className="text-foreground-alt size-4" />
          information_schema
        </span>
        <span className="text-foreground flex items-center gap-2 px-2 py-1">
          <LuDatabase className="text-brand size-4" />
          quickstart
        </span>
        <span className="bg-foreground/5 text-foreground flex items-center gap-2 rounded px-2 py-1 pl-8">
          <LuTable className="text-foreground-alt size-4" />
          people
        </span>
        <span className="text-foreground flex items-center gap-2 px-2 py-1 pl-8">
          <LuTable className="text-foreground-alt size-4" />
          projects
        </span>
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-3 p-4">
        <code className="border-foreground/10 text-foreground rounded border px-3 py-2 text-xs">
          {PLUGINS_DEMO_QUERY}
        </code>
        <table className="w-full text-left font-mono text-xs">
          <thead className="text-foreground-alt">
            <tr className="border-foreground/10 border-b">
              <th className="px-3 py-2 font-medium">name</th>
              <th className="px-3 py-2 font-medium">role</th>
            </tr>
          </thead>
          <tbody className="text-foreground">
            {rows.map((row) => (
              <tr key={row.name} className="border-foreground/5 border-b">
                <td className="px-3 py-2">{row.name}</td>
                <td className="px-3 py-2">{row.role}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
