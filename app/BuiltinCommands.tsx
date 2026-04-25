import { useCallback, useState } from 'react'

import { getAppPath, setAppPath } from '@s4wave/web/router/app-path.js'
import { useCommand } from '@s4wave/web/command/useCommand.js'
import { KeyboardShortcutsDialog } from '@s4wave/web/command/KeyboardShortcutsDialog.js'
import { AboutDialog } from '@s4wave/app/AboutDialog.js'
import { EmailSupportDialog } from '@s4wave/app/EmailSupportDialog.js'
import { DISCORD_INVITE_URL, GITHUB_ISSUES_URL } from '@s4wave/app/github.js'
import {
  addTab as addShellTab,
  useShellTabs,
} from '@s4wave/app/ShellTabContext.js'

// BuiltinCommands registers built-in commands with the command registry.
// Returns null (no UI).
export function BuiltinCommands() {
  const { tabs, activeTabId, setTabs, setActiveTabId } = useShellTabs()
  const [shortcutsOpen, setShortcutsOpen] = useState(false)
  const [aboutOpen, setAboutOpen] = useState(false)
  const [emailSupportOpen, setEmailSupportOpen] = useState(false)

  const openPathInNewTab = useCallback(
    (path: string) => {
      const currentPath = getAppPath()
      if (!currentPath.startsWith('/u/') && !currentPath.startsWith('/g/')) {
        setAppPath(path)
        return
      }
      const result = addShellTab(tabs, path, activeTabId || undefined)
      setTabs(result.tabs)
      setActiveTabId(result.newTab.id)
    },
    [tabs, activeTabId, setTabs, setActiveTabId],
  )

  useCommand({
    commandId: 'spacewave.view.fullscreen',
    label: 'Toggle Fullscreen',
    keybinding: 'F11',
    menuPath: 'View/Fullscreen',
    menuGroup: 20,
    menuOrder: 3,
    handler: useCallback(() => {
      if (document.fullscreenElement) {
        void document.exitFullscreen()
      } else {
        void document.documentElement.requestFullscreen()
      }
    }, []),
  })

  useCommand({
    commandId: 'spacewave.help.shortcuts',
    label: 'Keyboard Shortcuts',
    menuPath: 'Help/Keyboard Shortcuts',
    menuGroup: 10,
    menuOrder: 2,
    handler: useCallback(() => setShortcutsOpen(true), []),
  })

  useCommand({
    commandId: 'spacewave.help.about',
    label: 'About Spacewave',
    menuPath: 'Help/About',
    menuGroup: 90,
    menuOrder: 1,
    handler: useCallback(() => setAboutOpen(true), []),
  })

  useCommand({
    commandId: 'spacewave.help.docs',
    label: 'Documentation',
    menuPath: 'Help/Documentation',
    menuGroup: 10,
    menuOrder: 1,
    handler: useCallback(() => {
      openPathInNewTab('/docs')
    }, [openPathInNewTab]),
  })

  useCommand({
    commandId: 'spacewave.help.changelog',
    label: 'Changelog',
    menuPath: 'Help/Changelog',
    menuGroup: 10,
    menuOrder: 3,
    handler: useCallback(() => {
      setAppPath('/changelog')
    }, []),
  })

  useCommand({
    commandId: 'spacewave.help.report-issue',
    label: 'Report Issue',
    menuPath: 'Help/Report Issue',
    menuGroup: 10,
    menuOrder: 4,
    handler: useCallback(() => {
      window.open(GITHUB_ISSUES_URL, '_blank')
    }, []),
  })

  useCommand({
    commandId: 'spacewave.help.email-support',
    label: 'Email Support',
    menuPath: 'Help/Email Support',
    menuGroup: 10,
    menuOrder: 5,
    handler: useCallback(() => setEmailSupportOpen(true), []),
  })

  useCommand({
    commandId: 'spacewave.help.discord',
    label: 'Discord',
    menuPath: 'Help/Discord',
    menuGroup: 10,
    menuOrder: 6,
    handler: useCallback(() => {
      window.open(DISCORD_INVITE_URL, '_blank')
    }, []),
  })

  return (
    <>
      <KeyboardShortcutsDialog
        open={shortcutsOpen}
        onOpenChange={setShortcutsOpen}
      />
      <AboutDialog open={aboutOpen} onOpenChange={setAboutOpen} />
      <EmailSupportDialog
        open={emailSupportOpen}
        onOpenChange={setEmailSupportOpen}
      />
    </>
  )
}
