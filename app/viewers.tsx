import type { ObjectViewerComponent } from '@s4wave/web/object/object.js'
import {
  createViewerCatalog,
  getBaseObjectViewers,
} from '@s4wave/web/sdk/app/index.js'
import { DriveViewer } from '@s4wave/app/unixfs/DriveViewer.js'
import { UnixFSGalleryViewer } from '@s4wave/app/unixfs/UnixFSGalleryViewer.js'
import { UnixFSTypeID } from '@s4wave/sdk/unixfs/type.js'
import { GitRepoViewer, GitRepoTypeID } from '@s4wave/app/git/GitRepoViewer.js'
import {
  GitWorktreeViewer,
  GitWorktreeTypeID,
} from '@s4wave/app/git/GitWorktreeViewer.js'
import {
  CanvasViewer,
  CanvasTypeID,
} from '@s4wave/app/canvas/viewer/CanvasViewer.js'
import {
  ForgeTaskViewer,
  ForgeTaskTypeID,
} from '@s4wave/app/forge/ForgeTaskViewer.js'
import {
  ForgeJobViewer,
  ForgeJobTypeID,
} from '@s4wave/app/forge/ForgeJobViewer.js'
import {
  ForgeClusterViewer,
  ForgeClusterTypeID,
} from '@s4wave/app/forge/ForgeClusterViewer.js'
import {
  ForgeWorkerViewer,
  ForgeWorkerTypeID,
} from '@s4wave/app/forge/ForgeWorkerViewer.js'
import {
  ForgePassViewer,
  ForgePassTypeID,
} from '@s4wave/app/forge/ForgePassViewer.js'
import {
  ForgeExecutionViewer,
  ForgeExecutionTypeID,
} from '@s4wave/app/forge/ForgeExecutionViewer.js'
import {
  ForgeDashboardViewer,
  ForgeDashboardTypeID,
} from '@s4wave/app/forge/ForgeDashboardViewer.js'
import {
  ManifestViewer,
  ManifestTypeID,
} from '@s4wave/app/manifest/ManifestViewer.js'
import {
  ChatChannelViewer,
  ChatChannelTypeID,
} from '@s4wave/app/chat/ChatChannelViewer.js'
import {
  ChatMessageViewer,
  ChatMessageTypeID,
} from '@s4wave/app/chat/ChatMessageViewer.js'
import { DeviceViewer, DeviceTypeID } from '@s4wave/app/device/DeviceViewer.js'
import {
  SshHostViewer,
  SshHostTypeID,
} from '@s4wave/app/device/SshHostViewer.js'
import {
  ComputersDashboardViewer,
  ComputersDashboardTypeID,
} from '@s4wave/app/device/ComputersDashboardViewer.js'
import {
  AddDeviceWizardViewer,
  AddDeviceWizardTypeID,
} from '@s4wave/app/device/AddDeviceWizardViewer.js'
import {
  TerminalViewer,
  TerminalTypeID,
} from '@s4wave/app/terminal/TerminalViewer.js'
import { OrgViewer, OrganizationTypeID } from '@s4wave/app/org/OrgViewer.js'
import { SecretViewer, SecretTypeID } from '@s4wave/app/secret/SecretViewer.js'
import { KvStoreViewer, KvStoreTypeID } from '@s4wave/app/kv/KvStoreViewer.js'
import { SqlDbViewer, SqlDbTypeID } from '@s4wave/app/sql/SqlDbViewer.js'
import {
  SqlQueryViewer,
  SqlQueryTypeID,
} from '@s4wave/app/sql/SqlQueryViewer.js'
import {
  SqlQueryResultViewer,
  SqlQueryResultTypeID,
} from '@s4wave/app/sql/SqlQueryResultViewer.js'
import {
  SqlSchemaViewer,
  SqlSchemaTypeID,
} from '@s4wave/app/sql/SqlSchemaViewer.js'
import {
  SqlTableViewViewer,
  SqlTableViewTypeID,
} from '@s4wave/app/sql/SqlTableViewViewer.js'
import {
  SqlWorkbenchViewer,
  SqlWorkbenchTypeID,
} from '@s4wave/app/sql/SqlWorkbenchViewer.js'
import {
  WizardViewer,
  WizardTypePrefix,
} from '@s4wave/app/wizard/WizardViewer.js'
import {
  ForgeJobWizardViewer,
  ForgeJobWizardTypeID,
} from '@s4wave/app/wizard/ForgeJobWizardViewer.js'
import {
  ForgeTaskWizardViewer,
  ForgeTaskWizardTypeID,
} from '@s4wave/app/wizard/ForgeTaskWizardViewer.js'
import {
  GitRepoWizardViewer,
  GitRepoWizardTypeID,
} from '@s4wave/app/wizard/GitRepoWizardViewer.js'
import { IntroWizardViewer } from '@s4wave/app/wizard/IntroWizardViewer.js'
import { IntroWizardTypeID } from '@s4wave/app/wizard/intro.js'
import {
  VmV86WizardViewer,
  VmV86WizardTypeID,
} from '@s4wave/app/wizard/VmV86WizardViewer.js'
import { getViewersForType } from '@s4wave/web/hooks/useViewerRegistry.js'

const productObjectViewers: ObjectViewerComponent[] = [
  {
    componentID: 'spacewave.unixfs.viewer',
    typeID: UnixFSTypeID,
    name: 'UnixFS Viewer',
    category: 'Files',
    requiresObjectState: false,
    component: DriveViewer,
  },
  {
    componentID: 'spacewave.unixfs.gallery',
    typeID: UnixFSTypeID,
    name: 'UnixFS Gallery',
    category: 'Files',
    requiresObjectState: false,
    component: UnixFSGalleryViewer,
  },
  {
    componentID: 'spacewave.git.repo',
    typeID: GitRepoTypeID,
    name: 'Git Repo',
    category: 'Code',
    component: GitRepoViewer,
  },
  {
    componentID: 'spacewave.git.worktree',
    typeID: GitWorktreeTypeID,
    name: 'Git Worktree',
    category: 'Code',
    component: GitWorktreeViewer,
  },
  {
    componentID: 'spacewave.canvas.viewer',
    typeID: CanvasTypeID,
    name: 'Canvas',
    category: 'Layout',
    disablePadding: true,
    component: CanvasViewer,
  },
  {
    componentID: 'spacewave.forge.task',
    typeID: ForgeTaskTypeID,
    name: 'Task',
    category: 'Forge',
    component: ForgeTaskViewer,
  },
  {
    componentID: 'spacewave.forge.job',
    typeID: ForgeJobTypeID,
    name: 'Job',
    category: 'Forge',
    component: ForgeJobViewer,
  },
  {
    componentID: 'spacewave.forge.cluster',
    typeID: ForgeClusterTypeID,
    name: 'Cluster',
    category: 'Forge',
    component: ForgeClusterViewer,
  },
  {
    componentID: 'spacewave.forge.worker',
    typeID: ForgeWorkerTypeID,
    name: 'Worker',
    category: 'Forge',
    component: ForgeWorkerViewer,
  },
  {
    componentID: 'spacewave.forge.pass',
    typeID: ForgePassTypeID,
    name: 'Pass',
    category: 'Forge',
    component: ForgePassViewer,
  },
  {
    componentID: 'spacewave.forge.execution',
    typeID: ForgeExecutionTypeID,
    name: 'Execution',
    category: 'Forge',
    component: ForgeExecutionViewer,
  },
  {
    componentID: 'spacewave.forge.dashboard',
    typeID: ForgeDashboardTypeID,
    name: 'Forge Dashboard',
    category: 'Forge',
    component: ForgeDashboardViewer,
  },
  {
    componentID: 'spacewave.manifest.viewer',
    typeID: ManifestTypeID,
    name: 'Manifest',
    category: 'Build',
    component: ManifestViewer,
  },
  {
    componentID: 'spacewave.chat.channel',
    typeID: ChatChannelTypeID,
    name: 'Chat Channel',
    category: 'Chat',
    component: ChatChannelViewer,
  },
  {
    componentID: 'spacewave.chat.message',
    typeID: ChatMessageTypeID,
    name: 'Chat Message',
    category: 'Chat',
    component: ChatMessageViewer,
  },
  {
    componentID: 'spacewave.organization.viewer',
    typeID: OrganizationTypeID,
    name: 'Organization',
    category: 'Management',
    component: OrgViewer,
  },
  {
    componentID: 'spacewave.secret.viewer',
    typeID: SecretTypeID,
    name: 'Secret',
    category: 'Management',
    component: SecretViewer,
  },
  {
    componentID: 'spacewave.kv.store',
    typeID: KvStoreTypeID,
    name: 'Key/Value Store',
    category: 'Data',
    component: KvStoreViewer,
  },
  {
    componentID: 'spacewave.sql.db',
    typeID: SqlDbTypeID,
    name: 'SQL Database',
    category: 'Data',
    component: SqlDbViewer,
  },
  {
    componentID: 'spacewave.sql.query',
    typeID: SqlQueryTypeID,
    name: 'SQL Query',
    category: 'Data',
    component: SqlQueryViewer,
  },
  {
    componentID: 'spacewave.sql.query-result',
    typeID: SqlQueryResultTypeID,
    name: 'SQL Query Result',
    category: 'Data',
    component: SqlQueryResultViewer,
  },
  {
    componentID: 'spacewave.sql.schema',
    typeID: SqlSchemaTypeID,
    name: 'SQL Schema',
    category: 'Data',
    component: SqlSchemaViewer,
  },
  {
    componentID: 'spacewave.sql.table-view',
    typeID: SqlTableViewTypeID,
    name: 'SQL Table View',
    category: 'Data',
    component: SqlTableViewViewer,
  },
  {
    componentID: 'spacewave.sql.workbench',
    typeID: SqlWorkbenchTypeID,
    name: 'SQL Workbench',
    category: 'Data',
    component: SqlWorkbenchViewer,
  },
  {
    componentID: 'spacewave.device.viewer',
    typeID: DeviceTypeID,
    name: 'Device',
    category: 'Devices',
    component: DeviceViewer,
  },
  {
    componentID: 'spacewave.ssh-host.viewer',
    typeID: SshHostTypeID,
    name: 'SSH Host',
    category: 'Devices',
    component: SshHostViewer,
  },
  {
    componentID: 'spacewave.computers.viewer',
    typeID: ComputersDashboardTypeID,
    name: 'Computers',
    category: 'Devices',
    component: ComputersDashboardViewer,
  },
  {
    componentID: 'spacewave.terminal.viewer',
    typeID: TerminalTypeID,
    name: 'Terminal',
    category: 'Devices',
    component: TerminalViewer,
  },
  {
    componentID: 'spacewave.wizard.add-device',
    typeID: AddDeviceWizardTypeID,
    name: 'Add Device',
    category: 'Devices',
    requiresObjectState: false,
    component: AddDeviceWizardViewer,
  },
  {
    componentID: 'spacewave.wizard.forge-job',
    typeID: ForgeJobWizardTypeID,
    name: 'Job Wizard',
    category: 'Forge',
    requiresObjectState: false,
    component: ForgeJobWizardViewer,
  },
  {
    componentID: 'spacewave.wizard.forge-task',
    typeID: ForgeTaskWizardTypeID,
    name: 'Task Wizard',
    category: 'Forge',
    requiresObjectState: false,
    component: ForgeTaskWizardViewer,
  },
  {
    componentID: 'spacewave.wizard.git-repo',
    typeID: GitRepoWizardTypeID,
    name: 'Git Repo Wizard',
    category: 'Code',
    requiresObjectState: false,
    component: GitRepoWizardViewer,
  },
  {
    componentID: 'spacewave.wizard.intro',
    typeID: IntroWizardTypeID,
    name: 'New User Intro',
    category: 'System',
    requiresObjectState: false,
    component: IntroWizardViewer,
  },
  {
    componentID: 'spacewave.wizard.v86',
    typeID: VmV86WizardTypeID,
    name: 'V86 Wizard',
    category: 'VM',
    requiresObjectState: false,
    component: VmV86WizardViewer,
  },
  {
    componentID: 'spacewave.wizard.generic',
    typeID: WizardTypePrefix + '*',
    name: 'Wizard',
    category: 'System',
    requiresObjectState: false,
    component: WizardViewer,
  },
]

export function getProductObjectViewers(): ObjectViewerComponent[] {
  return [...productObjectViewers]
}

export function getObjectViewersForType(
  typeID: string,
  dynamicViewers?: ObjectViewerComponent[],
): ObjectViewerComponent[] {
  const all = getAllObjectViewers(dynamicViewers)
  return getViewersForType(typeID, all)
}

export function getAllObjectViewers(
  dynamicViewers?: ObjectViewerComponent[],
): ObjectViewerComponent[] {
  return createViewerCatalog({
    base: getBaseObjectViewers(),
    product: getProductObjectViewers(),
    downstream: dynamicViewers ?? [],
  })
}
