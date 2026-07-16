import { useCallback, useState } from 'react'
import { isDesktop, quitDesktopRuntime } from '@aptre/bldr'

import { getAppPath, setAppPath } from '@s4wave/web/router/app-path.js'
import { useCommand } from '@s4wave/web/command/useCommand.js'
import { KeyboardShortcutsDialog } from '@s4wave/web/command/KeyboardShortcutsDialog.js'
import {
  KeybindingEditor,
  type KeybindingEditorScope,
} from '@s4wave/web/command/KeybindingEditor.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { AboutDialog } from '@s4wave/app/AboutDialog.js'
import { EmailSupportDialog } from '@s4wave/app/EmailSupportDialog.js'
import { DISCORD_INVITE_URL, GITHUB_ISSUES_URL } from '@s4wave/app/github.js'
import { SPACEWAVE_PUBLIC_BASE_URL } from '@s4wave/app/urls.js'
import { useAddSpaceRootAlias } from '@s4wave/app/hooks/useAddSpaceRootAlias.js'
import { useShellTabs } from '@s4wave/app/ShellTabContext.js'
import { SelectAccountCommand } from '@s4wave/app/session/SelectAccountCommand.js'

// BuiltinCommands registers built-in commands with the command registry.
// Returns null (no UI).
export function BuiltinCommands() {
  const { activeTabId, openPathInNewTab, resetShellTabs } = useShellTabs()
  const [shortcutsOpen, setShortcutsOpen] = useState(false)
  const [keybindingEditorOpen, setKeybindingEditorOpen] = useState(false)
  const [keybindingEditorScope, setKeybindingEditorScope] =
    useState<KeybindingEditorScope>('local')
  const [keybindingEditorCommandId, setKeybindingEditorCommandId] = useState<
    string | undefined
  >()
  const [aboutOpen, setAboutOpen] = useState(false)
  const [emailSupportOpen, setEmailSupportOpen] = useState(false)
  const { add: addRootAlias, canAdd: canAddRootAlias } = useAddSpaceRootAlias()

  const handleOpenPathInNewTab = useCallback(
    (path: string) => {
      openPathInNewTab(path, {
        afterTabId: activeTabId || undefined,
        focusExisting: true,
      })
    },
    [activeTabId, openPathInNewTab],
  )

  const openKeybindingEditor = useCallback(
    (scope: KeybindingEditorScope = 'local', commandId?: string) => {
      setKeybindingEditorScope(scope)
      setKeybindingEditorCommandId(commandId)
      setKeybindingEditorOpen(true)
    },
    [],
  )

  useCommand({
    commandId: 'spacewave.root.add',
    label: 'Add State Root',
    description: 'Register an existing .spacewave state directory',
    active: isDesktop,
    enabled: canAddRootAlias,
    handler: useCallback(() => {
      void addRootAlias()
    }, [addRootAlias]),
  })

  useCommand({
    commandId: 'spacewave.shell.reset-tabs',
    label: 'Reset Shell Tabs',
    description: 'Replace the shared Shell Tab inventory with a fresh Home tab',
    menuPath: 'View/Reset Shell Tabs',
    menuGroup: 20,
    menuOrder: 4,
    handler: useCallback(() => resetShellTabs(), [resetShellTabs]),
  })

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
    commandId: 'spacewave.view.copy-link',
    label: 'Copy Link to Current View',
    description: 'Copy a public spacewave.app URL for the current view',
    menuPath: 'View/Copy Link',
    menuGroup: 10,
    menuOrder: 1,
    handler: useCallback(() => {
      const path = getAppPath()
      const url = `${SPACEWAVE_PUBLIC_BASE_URL}/#${path}`
      navigator.clipboard.writeText(url).then(
        () => {
          toast.success('Copied link', {
            description: url,
            duration: 2000,
          })
        },
        () => {
          toast.error('Copy failed', {
            description: 'Could not copy link to clipboard.',
          })
        },
      )
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
    commandId: 'spacewave.preferences.keyboard-shortcuts',
    label: 'Edit Keyboard Shortcuts',
    description: 'Customize local keyboard shortcuts',
    menuPath: 'Tools/Keyboard Shortcuts',
    menuGroup: 10,
    menuOrder: 1,
    handler: useCallback(
      (args: Record<string, string>) => {
        const scope =
          args.scope === 'account' || args.scope === 'space'
            ? args.scope
            : 'local'
        openKeybindingEditor(scope, args.commandId || undefined)
      },
      [openKeybindingEditor],
    ),
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
      handleOpenPathInNewTab('/docs')
    }, [handleOpenPathInNewTab]),
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
      <SelectAccountCommand />
      {isDesktop && <DesktopBuiltinCommands />}
      <KeyboardShortcutsDialog
        open={shortcutsOpen}
        onOpenChange={setShortcutsOpen}
        onEditCommand={(commandId) => {
          setShortcutsOpen(false)
          openKeybindingEditor('local', commandId)
        }}
      />
      <KeybindingEditor
        open={keybindingEditorOpen}
        onOpenChange={setKeybindingEditorOpen}
        initialScope={keybindingEditorScope}
        initialCommandId={keybindingEditorCommandId}
      />
      <AboutDialog open={aboutOpen} onOpenChange={setAboutOpen} />
      <EmailSupportDialog
        open={emailSupportOpen}
        onOpenChange={setEmailSupportOpen}
      />
    </>
  )
}

function DesktopBuiltinCommands() {
  useCommand({
    commandId: 'spacewave.file.close-window',
    label: 'Close Window',
    keybinding: 'CmdOrCtrl+W',
    handler: useCallback(() => {
      window.close()
    }, []),
  })

  useCommand({
    commandId: 'spacewave.file.quit',
    label: 'Quit',
    keybinding: 'CmdOrCtrl+Q',
    handler: useCallback(() => {
      quitDesktopRuntime().catch((err: unknown) => {
        console.error('Quit desktop runtime failed:', err)
        toast.error('Quit failed', {
          description: 'Could not request desktop shutdown.',
        })
      })
    }, []),
  })

  return null
}
