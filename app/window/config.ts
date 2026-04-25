export interface WorkspaceConfig {
  id: string
  name: string
}

export const DEFAULT_WORKSPACES: WorkspaceConfig[] = [
  { id: 'dashboard', name: 'Dashboard' },
  { id: 'my-drive', name: 'My Drive' },
  { id: 'chat', name: 'Chat' },
  { id: 'database', name: 'My Database' },
  { id: 'project', name: 'My Project' },
  { id: 'documents', name: 'Documents' },
  { id: 'calendar', name: 'Calendar' },
  { id: 'tasks', name: 'Tasks' },
  { id: 'notes', name: 'Notes' },
  { id: 'code', name: 'Code Editor' },
  { id: 'terminal', name: 'Terminal' },
]
