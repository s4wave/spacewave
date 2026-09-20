import { lazy } from 'react'

import type { ObjectViewerComponent } from '@s4wave/web/object/object.js'
import { getBaseObjectViewers } from '@s4wave/web/sdk/app/base-viewers.js'
import { createViewerCatalog } from '@s4wave/web/sdk/app/viewer-catalog.js'
import { getViewersForType } from '@s4wave/web/hooks/useViewerRegistry.js'

// Viewer metadata is available synchronously; implementations load only when
// ObjectViewerContent renders them within its existing Suspense boundary.
const productObjectViewers: ObjectViewerComponent[] = [
  {
    componentID: 'spacewave.unixfs.viewer',
    typeID: 'unixfs/fs-node',
    name: 'UnixFS Viewer',
    category: 'Files',
    requiresObjectState: false,
    component: lazy(() =>
      import('@s4wave/app/unixfs/DriveViewer.js').then((module) => ({
        default: module.DriveViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.unixfs.gallery',
    typeID: 'unixfs/fs-node',
    name: 'UnixFS Gallery',
    category: 'Files',
    requiresObjectState: false,
    component: lazy(() =>
      import('@s4wave/app/unixfs/UnixFSGalleryViewer.js').then((module) => ({
        default: module.UnixFSGalleryViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.git.repo',
    typeID: 'git/repo',
    name: 'Git Repo',
    category: 'Code',
    component: lazy(() =>
      import('@s4wave/app/git/GitRepoViewer.js').then((module) => ({
        default: module.GitRepoViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.git.worktree',
    typeID: 'git/worktree',
    name: 'Git Worktree',
    category: 'Code',
    component: lazy(() =>
      import('@s4wave/app/git/GitWorktreeViewer.js').then((module) => ({
        default: module.GitWorktreeViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.canvas.viewer',
    typeID: 'canvas',
    name: 'Canvas',
    category: 'Layout',
    disablePadding: true,
    component: lazy(() =>
      import('@s4wave/app/canvas/viewer/CanvasViewer.js').then((module) => ({
        default: module.CanvasViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.forge.task',
    typeID: 'forge/task',
    name: 'Task',
    category: 'Forge',
    component: lazy(() =>
      import('@s4wave/app/forge/ForgeTaskViewer.js').then((module) => ({
        default: module.ForgeTaskViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.forge.job',
    typeID: 'forge/job',
    name: 'Job',
    category: 'Forge',
    component: lazy(() =>
      import('@s4wave/app/forge/ForgeJobViewer.js').then((module) => ({
        default: module.ForgeJobViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.forge.cluster',
    typeID: 'forge/cluster',
    name: 'Cluster',
    category: 'Forge',
    component: lazy(() =>
      import('@s4wave/app/forge/ForgeClusterViewer.js').then((module) => ({
        default: module.ForgeClusterViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.forge.worker',
    typeID: 'forge/worker',
    name: 'Worker',
    category: 'Forge',
    component: lazy(() =>
      import('@s4wave/app/forge/ForgeWorkerViewer.js').then((module) => ({
        default: module.ForgeWorkerViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.forge.pass',
    typeID: 'forge/pass',
    name: 'Pass',
    category: 'Forge',
    component: lazy(() =>
      import('@s4wave/app/forge/ForgePassViewer.js').then((module) => ({
        default: module.ForgePassViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.forge.execution',
    typeID: 'forge/execution',
    name: 'Execution',
    category: 'Forge',
    component: lazy(() =>
      import('@s4wave/app/forge/ForgeExecutionViewer.js').then((module) => ({
        default: module.ForgeExecutionViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.forge.dashboard',
    typeID: 'spacewave/forge/dashboard',
    name: 'Forge Dashboard',
    category: 'Forge',
    component: lazy(() =>
      import('@s4wave/app/forge/ForgeDashboardViewer.js').then((module) => ({
        default: module.ForgeDashboardViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.manifest.viewer',
    typeID: 'bldr/manifest',
    name: 'Manifest',
    category: 'Build',
    component: lazy(() =>
      import('@s4wave/app/manifest/ManifestViewer.js').then((module) => ({
        default: module.ManifestViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.chat.channel',
    typeID: 'spacewave-chat/channel',
    name: 'Chat Channel',
    category: 'Chat',
    component: lazy(() =>
      import('@s4wave/app/chat/ChatChannelViewer.js').then((module) => ({
        default: module.ChatChannelViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.chat.message',
    typeID: 'spacewave-chat/message',
    name: 'Chat Message',
    category: 'Chat',
    component: lazy(() =>
      import('@s4wave/app/chat/ChatMessageViewer.js').then((module) => ({
        default: module.ChatMessageViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.organization.viewer',
    typeID: 'spacewave/organization',
    name: 'Organization',
    category: 'Management',
    component: lazy(() =>
      import('@s4wave/app/org/OrgViewer.js').then((module) => ({
        default: module.OrgViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.secret.viewer',
    typeID: 'spacewave/secret',
    name: 'Secret',
    category: 'Management',
    component: lazy(() =>
      import('@s4wave/app/secret/SecretViewer.js').then((module) => ({
        default: module.SecretViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.kv.store',
    typeID: 'kv/store',
    name: 'Key/Value Store',
    category: 'Data',
    component: lazy(() =>
      import('@s4wave/app/kv/KvStoreViewer.js').then((module) => ({
        default: module.KvStoreViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.sql.db',
    typeID: 'sql/db',
    name: 'SQL Database',
    category: 'Data',
    component: lazy(() =>
      import('@s4wave/app/sql/SqlDbViewer.js').then((module) => ({
        default: module.SqlDbViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.sql.query',
    typeID: 'sql/query',
    name: 'SQL Query',
    category: 'Data',
    component: lazy(() =>
      import('@s4wave/app/sql/SqlQueryViewer.js').then((module) => ({
        default: module.SqlQueryViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.sql.query-result',
    typeID: 'sql/query-result',
    name: 'SQL Query Result',
    category: 'Data',
    component: lazy(() =>
      import('@s4wave/app/sql/SqlQueryResultViewer.js').then((module) => ({
        default: module.SqlQueryResultViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.sql.schema',
    typeID: 'sql/schema',
    name: 'SQL Schema',
    category: 'Data',
    component: lazy(() =>
      import('@s4wave/app/sql/SqlSchemaViewer.js').then((module) => ({
        default: module.SqlSchemaViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.sql.table-view',
    typeID: 'sql/table-view',
    name: 'SQL Table View',
    category: 'Data',
    component: lazy(() =>
      import('@s4wave/app/sql/SqlTableViewViewer.js').then((module) => ({
        default: module.SqlTableViewViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.sql.workbench',
    typeID: 'sql/workbench',
    name: 'SQL Workbench',
    category: 'Data',
    component: lazy(() =>
      import('@s4wave/app/sql/SqlWorkbenchViewer.js').then((module) => ({
        default: module.SqlWorkbenchViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.device.viewer',
    typeID: 'spacewave/device',
    name: 'Device',
    category: 'Devices',
    component: lazy(() =>
      import('@s4wave/app/device/DeviceViewer.js').then((module) => ({
        default: module.DeviceViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.ssh-host.viewer',
    typeID: 'spacewave/ssh-host',
    name: 'SSH Host',
    category: 'Devices',
    component: lazy(() =>
      import('@s4wave/app/device/SshHostViewer.js').then((module) => ({
        default: module.SshHostViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.computers.viewer',
    typeID: 'spacewave/computers',
    name: 'Computers',
    category: 'Devices',
    component: lazy(() =>
      import('@s4wave/app/device/ComputersDashboardViewer.js').then(
        (module) => ({ default: module.ComputersDashboardViewer }),
      ),
    ),
  },
  {
    componentID: 'spacewave.terminal.viewer',
    typeID: 'spacewave/terminal',
    name: 'Terminal',
    category: 'Devices',
    component: lazy(() =>
      import('@s4wave/app/terminal/TerminalViewer.js').then((module) => ({
        default: module.TerminalViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.wizard.add-device',
    typeID: 'wizard/device/add',
    name: 'Add Device',
    category: 'Devices',
    requiresObjectState: false,
    component: lazy(() =>
      import('@s4wave/app/device/AddDeviceWizardViewer.js').then((module) => ({
        default: module.AddDeviceWizardViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.wizard.forge-job',
    typeID: 'wizard/forge/job',
    name: 'Job Wizard',
    category: 'Forge',
    requiresObjectState: false,
    component: lazy(() =>
      import('@s4wave/app/wizard/ForgeJobWizardViewer.js').then((module) => ({
        default: module.ForgeJobWizardViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.wizard.forge-task',
    typeID: 'wizard/forge/task',
    name: 'Task Wizard',
    category: 'Forge',
    requiresObjectState: false,
    component: lazy(() =>
      import('@s4wave/app/wizard/ForgeTaskWizardViewer.js').then((module) => ({
        default: module.ForgeTaskWizardViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.wizard.git-repo',
    typeID: 'wizard/git/repo',
    name: 'Git Repo Wizard',
    category: 'Code',
    requiresObjectState: false,
    component: lazy(() =>
      import('@s4wave/app/wizard/GitRepoWizardViewer.js').then((module) => ({
        default: module.GitRepoWizardViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.wizard.intro',
    typeID: 'wizard/intro',
    name: 'New User Intro',
    category: 'System',
    requiresObjectState: false,
    component: lazy(() =>
      import('@s4wave/app/wizard/IntroWizardViewer.js').then((module) => ({
        default: module.IntroWizardViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.wizard.v86',
    typeID: 'wizard/vm/v86',
    name: 'V86 Wizard',
    category: 'VM',
    requiresObjectState: false,
    component: lazy(() =>
      import('@s4wave/app/wizard/VmV86WizardViewer.js').then((module) => ({
        default: module.VmV86WizardViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.wizard.generic',
    typeID: 'wizard/*',
    name: 'Wizard',
    category: 'System',
    requiresObjectState: false,
    component: lazy(() =>
      import('@s4wave/app/wizard/WizardViewer.js').then((module) => ({
        default: module.WizardViewer,
      })),
    ),
  },
]

/** getProductObjectViewers returns the product catalog in default selection order. */
export function getProductObjectViewers(): ObjectViewerComponent[] {
  return [...productObjectViewers]
}

/** getObjectViewersForType orders matching product and plugin viewers. */
export function getObjectViewersForType(
  typeID: string,
  dynamicViewers?: ObjectViewerComponent[],
): ObjectViewerComponent[] {
  const all = getAllObjectViewers(dynamicViewers)
  return getViewersForType(typeID, all)
}

/** getAllObjectViewers combines built-in and plugin metadata without loading viewers. */
export function getAllObjectViewers(
  dynamicViewers?: ObjectViewerComponent[],
): ObjectViewerComponent[] {
  return createViewerCatalog({
    base: getBaseObjectViewers(),
    product: getProductObjectViewers(),
    downstream: dynamicViewers ?? [],
  })
}
