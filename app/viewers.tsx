import type { ObjectViewerComponent } from '@s4wave/web/object/object.js'
import { DebugObjectViewer } from '@s4wave/web/object/DebugObjectViewer.js'
import {
  LayoutObjectViewer,
  ObjectLayoutTypeID,
} from '@s4wave/web/object/LayoutObjectViewer.js'
import { UnixFSViewer, UnixFSTypeID } from '@s4wave/app/unixfs/UnixFSViewer.js'
import { UnixFSGalleryViewer } from '@s4wave/app/unixfs/UnixFSGalleryViewer.js'
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
import { VmV86Viewer, VmV86TypeID } from '@s4wave/app/vm/VmV86Viewer.js'
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
import {
  NotebookViewer,
  NotebookTypeID,
} from '@s4wave/app/notes/NotebookViewer.js'
import { BlogViewer, BlogTypeID } from '@s4wave/app/notes/BlogViewer.js'
import { DocsViewer, DocsTypeID } from '@s4wave/app/notes/DocsViewer.js'
import { OrgViewer, OrganizationTypeID } from '@s4wave/app/org/OrgViewer.js'
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
import {
  VmV86WizardViewer,
  VmV86WizardTypeID,
} from '@s4wave/app/wizard/VmV86WizardViewer.js'
import { getViewersForType } from '@s4wave/web/hooks/useViewerRegistry.js'

const staticViewers: ObjectViewerComponent[] = [
  {
    typeID: ObjectLayoutTypeID,
    name: 'Layout Viewer',
    category: 'Layout',
    component: LayoutObjectViewer,
  },
  {
    typeID: UnixFSTypeID,
    name: 'UnixFS Viewer',
    category: 'Files',
    component: UnixFSViewer,
  },
  {
    typeID: UnixFSTypeID,
    name: 'UnixFS Gallery',
    category: 'Files',
    component: UnixFSGalleryViewer,
  },
  {
    typeID: GitRepoTypeID,
    name: 'Git Repo',
    category: 'Code',
    component: GitRepoViewer,
  },
  {
    typeID: GitWorktreeTypeID,
    name: 'Git Worktree',
    category: 'Code',
    component: GitWorktreeViewer,
  },
  {
    typeID: CanvasTypeID,
    name: 'Canvas',
    category: 'Layout',
    disablePadding: true,
    component: CanvasViewer,
  },
  {
    typeID: ForgeTaskTypeID,
    name: 'Task',
    category: 'Forge',
    component: ForgeTaskViewer,
  },
  {
    typeID: ForgeJobTypeID,
    name: 'Job',
    category: 'Forge',
    component: ForgeJobViewer,
  },
  {
    typeID: ForgeClusterTypeID,
    name: 'Cluster',
    category: 'Forge',
    component: ForgeClusterViewer,
  },
  {
    typeID: ForgeWorkerTypeID,
    name: 'Worker',
    category: 'Forge',
    component: ForgeWorkerViewer,
  },
  {
    typeID: ForgePassTypeID,
    name: 'Pass',
    category: 'Forge',
    component: ForgePassViewer,
  },
  {
    typeID: ForgeExecutionTypeID,
    name: 'Execution',
    category: 'Forge',
    component: ForgeExecutionViewer,
  },
  {
    typeID: ForgeDashboardTypeID,
    name: 'Forge Dashboard',
    category: 'Forge',
    component: ForgeDashboardViewer,
  },
  {
    typeID: VmV86TypeID,
    name: 'V86',
    category: 'VM',
    component: VmV86Viewer,
  },
  {
    typeID: ManifestTypeID,
    name: 'Manifest',
    category: 'Build',
    component: ManifestViewer,
  },
  {
    typeID: ChatChannelTypeID,
    name: 'Chat Channel',
    category: 'Chat',
    component: ChatChannelViewer,
  },
  {
    typeID: ChatMessageTypeID,
    name: 'Chat Message',
    category: 'Chat',
    component: ChatMessageViewer,
  },
  {
    typeID: NotebookTypeID,
    name: 'Notebook',
    category: 'Content',
    component: NotebookViewer,
  },
  {
    typeID: BlogTypeID,
    name: 'Blog',
    category: 'Content',
    component: BlogViewer,
  },
  {
    typeID: DocsTypeID,
    name: 'Documentation',
    category: 'Content',
    component: DocsViewer,
  },
  {
    typeID: OrganizationTypeID,
    name: 'Organization',
    category: 'Management',
    component: OrgViewer,
  },
  {
    typeID: ForgeJobWizardTypeID,
    name: 'Job Wizard',
    category: 'Forge',
    component: ForgeJobWizardViewer,
  },
  {
    typeID: ForgeTaskWizardTypeID,
    name: 'Task Wizard',
    category: 'Forge',
    component: ForgeTaskWizardViewer,
  },
  {
    typeID: GitRepoWizardTypeID,
    name: 'Git Repo Wizard',
    category: 'Code',
    component: GitRepoWizardViewer,
  },
  {
    typeID: VmV86WizardTypeID,
    name: 'V86 Wizard',
    category: 'VM',
    component: VmV86WizardViewer,
  },
  {
    typeID: WizardTypePrefix + '*',
    name: 'Wizard',
    category: 'System',
    component: WizardViewer,
  },
  {
    typeID: '*',
    name: 'Debug Viewer',
    category: 'Developer',
    component: DebugObjectViewer,
  },
]

export function getObjectViewersForType(
  typeID: string,
  dynamicViewers?: ObjectViewerComponent[],
): ObjectViewerComponent[] {
  const all =
    dynamicViewers ? [...staticViewers, ...dynamicViewers] : staticViewers
  return getViewersForType(typeID, all)
}

export function getAllObjectViewers(
  dynamicViewers?: ObjectViewerComponent[],
): ObjectViewerComponent[] {
  return dynamicViewers ?
      [...staticViewers, ...dynamicViewers]
    : [...staticViewers]
}
