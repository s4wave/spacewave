import { ForgeTaskCreateOp } from '@s4wave/core/forge/task/task.pb.js'
import { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { CreateGitRepoWizardOp } from '@s4wave/core/git/git.pb.js'

import type { StaticConfigTypeRegistration } from '@s4wave/web/configtype/configtype.js'
import { ForgeTaskConfigEditor } from './configeditors/ForgeTaskConfigEditor.js'
import { ForgeJobConfigEditor } from './configeditors/ForgeJobConfigEditor.js'
import { GitRepoConfigEditor } from './configeditors/GitRepoConfigEditor.js'

// staticConfigTypes is the list of built-in config type editors.
// Plugins can register additional config types dynamically via SRPC.
export const staticConfigTypes: StaticConfigTypeRegistration[] = [
  {
    configId: 'forge/task',
    displayName: 'Forge Task',
    category: 'Forge',
    messageType: ForgeTaskCreateOp,
    component: ForgeTaskConfigEditor,
  },
  {
    configId: 'forge/job',
    displayName: 'Forge Job',
    category: 'Forge',
    messageType: ForgeJobCreateOp,
    component: ForgeJobConfigEditor,
  },
  {
    configId: 'git/repo',
    displayName: 'Git Repository',
    category: 'Code',
    messageType: CreateGitRepoWizardOp,
    component: GitRepoConfigEditor,
  },
]
