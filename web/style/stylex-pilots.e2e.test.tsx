import { act, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { createPortal } from 'react-dom'
import * as stylex from '@stylexjs/stylex'
import { expect, it, onTestFinished } from 'vitest'
import { page, userEvent } from 'vitest/browser'

import { VmErrorBanner } from '../../app/vm/VmErrorBanner.js'
import { TagChip } from '../../plugin/notes/blog/TagChip.js'
import { InfoCard } from '../ui/InfoCard.js'
import { OutputPanel } from '../ui/OutputPanel.js'
import { CopyableField } from '../ui/CopyableField.js'
import { RadioOption } from '../ui/RadioOption.js'
import { Window } from '../ui/Window.js'
import { Input } from '../ui/input.js'
import { Label } from '../ui/label.js'
import { groupMarker, peerMarker } from './relations.stylex.js'
import {
  Trace,
  TraceLine,
  TraceTitle,
  TraceCollapsible,
  TraceCollapsibleTrigger,
  TraceCollapsibleContent,
} from '../ui/trace.js'
import { PathInput } from '../ui/path/PathInput.js'
import {
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  tabsContentItemStyles,
} from '../ui/tabs.js'
import { Button, buttonVariants } from '../ui/button.js'
import { DashboardButton } from '../ui/DashboardButton.js'
import { DropdownTriggerButton } from '../ui/DropdownTriggerButton.js'
import { IconContext, IconStylesProvider, LuPlus } from '../images/icons.js'
import { PanelHeader, PanelHeaderButton } from '../ui/PanelHeader.js'
import {
  Sheet,
  SheetTrigger,
  SheetClose,
  SheetContent,
  SheetHeader,
  SheetFooter,
  SheetTitle,
  SheetDescription,
} from '../ui/sheet.js'
import {
  Dialog,
  DialogTrigger,
  DialogClose,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from '../ui/dialog.js'
import {
  CollapsibleSection,
  collapsibleSectionActionStyles,
} from '../ui/CollapsibleSection.js'

const styles = stylex.create({
  override: { padding: 19, borderRadius: 9 },
  headerOverride: { borderRadius: 9, paddingBlock: 19, paddingInline: 19 },
  firstRule: { color: 'rgb(17, 34, 51)' },
  secondRule: { color: 'rgb(68, 85, 102)' },
  largeIcon: { height: 32, width: 32 },
  iconDefaults: { color: 'rgb(68, 85, 102)', height: 20, width: 20 },
  minimumWidth: (minWidth: number) => ({ minWidth }),
  pathOverride: { marginTop: 19 },
})

// IconScopeSamples covers default composition and a control's portal boundary.
function IconScopeSamples() {
  const [host, setHost] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <IconStylesProvider sx={styles.iconDefaults}>
        <LuPlus aria-label="Default native icon" />
        <LuPlus
          aria-label="Overridden native icon"
          sx={[styles.largeIcon, styles.firstRule]}
          style={{ '--native-icon-test': 'retained' }}
        />
      </IconStylesProvider>
      <div ref={setHost} aria-label="Portal icon host" />
      <IconContext.Provider value={{ color: 'rgb(17, 34, 51)' }}>
        <Button>
          Portal icon owner
          {host &&
            createPortal(
              <LuPlus
                aria-label="Portaled native icon"
                sx={styles.largeIcon}
              />,
              host,
            )}
        </Button>
      </IconContext.Provider>
    </>
  )
}

// PilotJourney exercises shared composition together with real app and plugin consumers.
function PilotJourney() {
  // Keep the fixture's independent interactions local to their mounted controls.
  const [selected, setSelected] = useState('None')
  const [radioSelected, setRadioSelected] = useState(false)
  const [sectionOpen, setSectionOpen] = useState(false)
  const [actions, setActions] = useState(0)
  const [sheetOpen, setSheetOpen] = useState(false)
  const [buttonActions, setButtonActions] = useState(0)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [path, setPath] = useState('/Users/example/Documents')

  return (
    <main>
      <div sx={groupMarker} data-disabled="true">
        <Label data-testid="ancestor-label">Disabled ancestor label</Label>
      </div>
      <div>
        <Label data-testid="preceding-label">Preceding label</Label>
        <input type="checkbox" disabled sx={peerMarker} />
        <Label data-testid="following-label">Following label</Label>
      </div>
      <input id="native-label-choice" type="checkbox" />
      <Label
        htmlFor="native-label-choice"
        sx={styles.minimumWidth(123)}
        style={{ '--native-label-test': 'retained' }}
      >
        Native choice
      </Label>
      <Trace>
        <TraceCollapsible>
          <TraceCollapsibleTrigger>
            <TraceLine status="done">
              <TraceTitle>Native trace</TraceTitle>
            </TraceLine>
          </TraceCollapsibleTrigger>
          <TraceCollapsibleContent>Trace detail</TraceCollapsibleContent>
        </TraceCollapsible>
      </Trace>
      <IconContext.Provider value={{ color: 'rgb(17, 34, 51)' }}>
        <Button
          sx={[styles.headerOverride, styles.minimumWidth(123)]}
          style={{ '--native-button-test': 'retained' }}
          onClick={() => setButtonActions((value) => value + 1)}
        >
          <LuPlus sx={styles.largeIcon} />
          Native action
        </Button>
        <Button asChild>
          <a href="#native-link">
            <LuPlus />
            Native link
          </a>
        </Button>
        <DashboardButton icon={<LuPlus />}>
          Native dashboard action
        </DashboardButton>
        <DropdownTriggerButton triggerStyle="ghost" icon={<LuPlus />}>
          Native dropdown action
        </DropdownTriggerButton>
      </IconContext.Provider>
      <IconScopeSamples />
      <button
        type="button"
        sx={buttonVariants({ variant: 'outline', size: 'sm' })}
      >
        Recipe action
      </button>
      <output aria-label="Native button actions">{buttonActions}</output>
      <Tabs defaultValue="first" variant="debug">
        <TabsList variant="debug">
          <TabsTrigger value="first" variant="debug" sx={styles.headerOverride}>
            Native first tab
          </TabsTrigger>
          <TabsTrigger value="disabled" variant="debug" disabled>
            Native disabled tab
          </TabsTrigger>
          <TabsTrigger value="second" variant="debug">
            Native second tab
          </TabsTrigger>
        </TabsList>
        <TabsContent value="first" variant="form">
          <p sx={tabsContentItemStyles.item}>First native panel</p>
          <div>Panel details</div>
        </TabsContent>
        <TabsContent value="second" variant="formTight">
          <p sx={tabsContentItemStyles.item}>Second native panel</p>
          <div>Panel details</div>
        </TabsContent>
      </Tabs>
      <PathInput
        path={path}
        onPathChange={setPath}
        onNavigate={setPath}
        sx={styles.pathOverride}
      />
      <Input
        aria-label="Native input"
        variant="config"
        placeholder="Field value"
        sx={styles.headerOverride}
      />
      <Input aria-label="Disabled native input" disabled />
      <InfoCard compact title="Details" sx={styles.override}>
        <VmErrorBanner title="Unavailable" message="Try again" />
        <TagChip tag="native-css" onSelectTag={setSelected} />
        <output aria-label="Selected tag">{selected}</output>
      </InfoCard>
      <section aria-label="Output rule precedence">
        <OutputPanel
          lines={['INFO WARN overlap', 'WARN predicate', 'Plain fallback']}
          rules={[
            { match: 'INFO', sx: styles.firstRule },
            { match: (line) => line.includes('WARN'), sx: styles.secondRule },
          ]}
        />
      </section>
      <CopyableField label="Identifier" value="shared-value" />
      <RadioOption
        label="Style choice"
        selected={radioSelected}
        onSelect={() => setRadioSelected(true)}
      />
      <Window
        data-area-id="fixture-window"
        sx={styles.minimumWidth(123)}
        style={{ '--native-window-test': 'retained' }}
      >
        <button type="button">Window action</button>
      </Window>
      <CollapsibleSection
        title="Expandable section"
        open={sectionOpen}
        onOpenChange={setSectionOpen}
        sx={styles.override}
        headerActions={
          <button
            type="button"
            sx={collapsibleSectionActionStyles.button}
            onClick={() => setActions(actions + 1)}
            aria-label="Section action"
          >
            +
          </button>
        }
      >
        <p>Expanded content</p>
      </CollapsibleSection>
      <output aria-label="Section actions">{actions}</output>
      <PanelHeader sx={styles.headerOverride}>
        <PanelHeaderButton title="Panel action" sx={styles.override}>
          Panel action
        </PanelHeaderButton>
      </PanelHeader>
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogTrigger sx={styles.override}>Open native dialog</DialogTrigger>
        <DialogContent
          sx={[styles.headerOverride, styles.minimumWidth(123)]}
          style={{ '--native-dialog-test': 'retained' }}
          className={undefined}
        >
          <DialogHeader>
            <DialogTitle>Native dialog proof</DialogTitle>
            <DialogDescription>Review the native dialog</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose sx={styles.override}>Close native dialog</DialogClose>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
        <SheetTrigger sx={styles.override}>Open sheet</SheetTrigger>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>Sheet proof</SheetTitle>
            <SheetDescription>Review the native sheet</SheetDescription>
          </SheetHeader>
          <p>Sheet body</p>
          <SheetFooter>
            <SheetClose sx={styles.override}>Done</SheetClose>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </main>
  )
}

it('composes shared, app and plugin styles through the browser build', async () => {
  // Render production components through Vitest's normal React and native Vite CSS pipeline.
  const previousAct = Reflect.get(globalThis, 'IS_REACT_ACT_ENVIRONMENT')
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  onTestFinished(() => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    act(() => root.unmount())
    host.remove()
    Reflect.set(globalThis, 'IS_REACT_ACT_ENVIRONMENT', previousAct)
  })
  await act(() => root.render(<PilotJourney />))
  Reflect.set(globalThis, 'IS_REACT_ACT_ENVIRONMENT', previousAct)

  // Observe the app and plugin content after the production React mount.
  await expect
    .poll(() => document.querySelector('[role=alert]')?.textContent)
    .toBe('UnavailableTry again')
  const card = page
    .getByRole('heading', { name: 'Details', exact: true })
    .element().parentElement!
  const style = getComputedStyle(card)
  expect(style.padding).toBe('19px')
  expect(style.borderRadius).toBe('9px')

  // Native label activation and marker conditions retain their direction and caller properties.
  const ancestorLabel = document.querySelector(
    '[data-testid="ancestor-label"]',
  )!
  expect(getComputedStyle(ancestorLabel).opacity).toBe('0.5')
  expect(getComputedStyle(ancestorLabel).pointerEvents).toBe('none')

  // Only a preceding disabled peer changes the label's state.
  expect(
    getComputedStyle(document.querySelector('[data-testid="preceding-label"]')!)
      .opacity,
  ).toBe('1')
  expect(
    getComputedStyle(document.querySelector('[data-testid="following-label"]')!)
      .opacity,
  ).toBe('0.5')

  // Native activation retains the caller's dynamic width and inline property.
  const label = page.getByText('Native choice', { exact: true })
  expect(getComputedStyle(label.element()).minWidth).toBe('123px')
  expect(
    getComputedStyle(label.element()).getPropertyValue('--native-label-test'),
  ).toBe('retained')
  await label.click()
  await expect
    .element(page.getByRole('checkbox', { name: 'Native choice', exact: true }))
    .toBeChecked()

  // The actual trace trigger drives expansion and its marked indicator together.
  const trace = page.getByRole('button', { name: 'Native trace', exact: true })
  const indicator = trace.element().querySelector('[data-slot="trace-icon"]')!
  expect(getComputedStyle(indicator).rotate).toBe('none')

  // Opening the trace rotates its indicator and reveals the nested detail.
  await trace.click()
  await expect.element(trace).toHaveAttribute('aria-expanded', 'true')
  await expect.poll(() => getComputedStyle(indicator).rotate).toBe('90deg')
  await expect
    .element(page.getByText('Trace detail', { exact: true }))
    .toBeVisible()

  // The same trigger returns to its closed state.
  await trace.click()
  await expect.element(trace).toHaveAttribute('aria-expanded', 'false')

  // A plugin interaction retains the same shared card and its caller's final override.
  await page.getByRole('button', { name: 'native-css', exact: true }).click()
  await expect
    .poll(
      () =>
        document.querySelector('output[aria-label="Selected tag"]')
          ?.textContent,
    )
    .toBe('native-css')
  expect(
    page.getByRole('heading', { name: 'Details', exact: true }).element()
      .parentElement,
  ).toBe(card)
  expect(getComputedStyle(card).padding).toBe('19px')

  // Prefix matching wins the overlap, while the next line reaches the predicate rule.
  const lines = document.querySelectorAll(
    'section[aria-label="Output rule precedence"] > div > div',
  )
  expect(lines).toHaveLength(3)
  expect(getComputedStyle(lines[0]!).color).toBe('rgb(17, 34, 51)')
  expect(getComputedStyle(lines[1]!).color).toBe('rgb(68, 85, 102)')

  // Resolve the fallback color through the browser's actual theme variables.
  const reference = document.createElement('span')
  reference.style.color = 'var(--color-text-secondary)'
  document.body.append(reference)
  expect(getComputedStyle(lines[2]!).color).toBe(
    getComputedStyle(reference).color,
  )
  reference.remove()

  // The copy icon follows the actual hover state of its marked button.
  const copyIcon = document.querySelector(
    'button[aria-label="Click to copy"] svg',
  )!
  expect(getComputedStyle(copyIcon).opacity).toBe('0')
  await page.getByRole('button', { name: 'Click to copy', exact: true }).hover()
  await expect.poll(() => getComputedStyle(copyIcon).opacity).toBe('1')

  // A real hover and selection change reach distinct radio border states.
  const radio = document.querySelector('[role="radio"]')!
  const idleBorder = getComputedStyle(radio).borderColor
  await page.getByRole('radio', { name: 'Style choice', exact: true }).hover()
  await expect
    .poll(() => getComputedStyle(radio).borderColor)
    .not.toBe(idleBorder)
  await page.getByRole('radio', { name: 'Style choice', exact: true }).click()
  expect(radio.getAttribute('aria-checked')).toBe('true')

  // The existing frame hover event updates its owned border without replacing the frame.
  const frame = document.querySelector('[data-area-id="fixture-window"]')!
  expect(getComputedStyle(frame).minWidth).toBe('123px')
  expect(getComputedStyle(frame).getPropertyValue('--native-window-test')).toBe(
    'retained',
  )
  const border = getComputedStyle(frame).borderColor
  await page.getByRole('button', { name: 'Window action', exact: true }).hover()
  await expect.poll(() => getComputedStyle(frame).borderColor).not.toBe(border)

  // The separate header action leaves expansion alone, while the trigger opens and closes content.
  const trigger = page.getByRole('button', {
    name: 'Expandable section',
    exact: true,
  })
  await page
    .getByRole('button', { name: 'Section action', exact: true })
    .click()
  expect(
    document.querySelector('output[aria-label="Section actions"]')?.textContent,
  ).toBe('1')
  await expect.element(trigger).toHaveAttribute('aria-expanded', 'false')

  // Expansion reveals content while retaining the caller's outer override.
  await trigger.click()
  await expect.element(trigger).toHaveAttribute('aria-expanded', 'true')
  await expect
    .element(page.getByText('Expanded content', { exact: true }))
    .toBeVisible()
  const section = document.querySelector('section[data-state="open"]')!
  expect(getComputedStyle(section).padding).toBe('19px')

  // Closing the same section removes its content from the visible surface.
  await trigger.click()
  await expect.element(trigger).toHaveAttribute('aria-expanded', 'false')
  await expect
    .poll(
      () =>
        page
          .getByText('Expanded content', { exact: true })
          .query()
          ?.checkVisibility() ?? false,
    )
    .toBe(false)

  // Keyboard focus reaches the header button's own outline and both caller overrides.
  await userEvent.keyboard('{Tab}')
  expect(document.activeElement?.getAttribute('aria-label')).toBe(
    'Section action',
  )
  await userEvent.keyboard('{Tab}')
  const panelButton = document.querySelector('button[title="Panel action"]')!
  expect(document.activeElement).toBe(panelButton)
  expect(getComputedStyle(panelButton).outlineWidth).toBe('2px')
  expect(getComputedStyle(panelButton).outlineStyle).toBe('solid')

  // Resolve the owned focus color through the real theme rather than a serialized color spelling.
  reference.style.color = 'var(--color-brand)'
  document.body.append(reference)
  await expect
    .poll(() => getComputedStyle(panelButton).outlineColor)
    .toBe(getComputedStyle(reference).color)
  reference.remove()

  // The migrated dialog retains trigger, footer and Escape behavior with caller overrides.
  const modalTrigger = page.getByRole('button', {
    name: 'Open native dialog',
    exact: true,
  })
  await modalTrigger.click()
  const modal = page.getByRole('dialog', {
    name: 'Native dialog proof',
    exact: true,
  })
  await expect.element(modal).toBeVisible()
  expect(getComputedStyle(modal.element()).padding).toBe('19px')

  // Native inline properties preserve the caller's dynamic sx variables.
  expect(getComputedStyle(modal.element()).minWidth).toBe('123px')
  expect(
    getComputedStyle(modal.element()).getPropertyValue('--native-dialog-test'),
  ).toBe('retained')

  // The footer action retains its caller's padding.
  const modalClose = page.getByRole('button', {
    name: 'Close native dialog',
    exact: true,
  })
  expect(getComputedStyle(modalClose.element()).padding).toBe('19px')

  // The footer dismisses the dialog and restores the trigger focus.
  await modalClose.click()
  await expect.poll(() => modal.query()?.checkVisibility() ?? false).toBe(false)
  await expect.poll(() => document.activeElement).toBe(modalTrigger.element())

  // Reopening retains the standard Escape close path.
  await modalTrigger.click()
  await expect.element(modal).toBeVisible()
  await userEvent.keyboard('{Escape}')
  await expect.poll(() => modal.query()?.checkVisibility() ?? false).toBe(false)

  // The two owned inputs keep the caller's final padding override.
  expect(getComputedStyle(panelButton).padding).toBe('19px')
  expect(getComputedStyle(panelButton.parentElement!).padding).toBe('19px')

  // The real modal opens from its trigger and keeps both action overrides.
  const sheetTrigger = page.getByRole('button', {
    name: 'Open sheet',
    exact: true,
  })
  await sheetTrigger.click()
  const dialog = page.getByRole('dialog', { name: 'Sheet proof', exact: true })
  await expect.element(dialog).toBeVisible()
  expect(getComputedStyle(dialog.element()).position).toBe('fixed')
  const done = page.getByRole('button', { name: 'Done', exact: true })
  expect(getComputedStyle(done.element()).padding).toBe('19px')

  // The footer closes the modal and returns focus to its trigger.
  await done.click()
  await expect
    .poll(() => dialog.query()?.checkVisibility() ?? false)
    .toBe(false)
  await expect.poll(() => document.activeElement).toBe(sheetTrigger.element())
  expect(getComputedStyle(sheetTrigger.element()).padding).toBe('19px')

  // A second open also closes through the ordinary Escape path.
  await sheetTrigger.click()
  await expect.element(dialog).toBeVisible()
  await userEvent.keyboard('{Escape}')
  await expect
    .poll(() => dialog.query()?.checkVisibility() ?? false)
    .toBe(false)

  // Native field behavior and compiled caller styles survive ordinary input events.
  const field = page.getByRole('textbox', { name: 'Native input', exact: true })
  await field.fill('Written value')
  await expect.element(field).toHaveValue('Written value')
  expect(getComputedStyle(field.element()).padding).toBe('19px')
  await expect
    .element(
      page.getByRole('textbox', { name: 'Disabled native input', exact: true }),
    )
    .toBeDisabled()

  // Keyboard tab activation skips disabled triggers and retains panel layout.
  const firstTab = page.getByRole('tab', {
    name: 'Native first tab',
    exact: true,
  })
  const secondTab = page.getByRole('tab', {
    name: 'Native second tab',
    exact: true,
  })
  await expect.element(firstTab).toHaveAttribute('aria-selected', 'true')
  expect(getComputedStyle(firstTab.element()).padding).toBe('19px')
  await expect
    .element(
      page.getByRole('tab', { name: 'Native disabled tab', exact: true }),
    )
    .toBeDisabled()

  // Moving right skips the disabled tab and reveals its associated form panel.
  await firstTab.click()
  await userEvent.keyboard('{ArrowRight}')
  await expect.element(secondTab).toHaveAttribute('aria-selected', 'true')
  await expect
    .element(
      page.getByRole('tabpanel', { name: 'Native second tab', exact: true }),
    )
    .toBeVisible()
  expect(
    getComputedStyle(
      page
        .getByRole('tabpanel', { name: 'Native second tab', exact: true })
        .element().firstElementChild!,
    ).marginBlockEnd,
  ).toBe('12px')

  // Breadcrumb edits commit with Enter, discard with Escape and navigate through their owner.
  await page.getByRole('button', { name: 'Edit path', exact: true }).click()
  const pathEditor = page.getByRole('textbox', {
    name: 'File path',
    exact: true,
  })
  await pathEditor.fill('/Users/example/Projects')
  await userEvent.keyboard('{Enter}')
  await expect
    .element(
      page.getByRole('button', { name: 'Navigate to Projects', exact: true }),
    )
    .toBeVisible()

  // Discard the second draft without changing the committed path.
  await page.getByRole('button', { name: 'Edit path', exact: true }).click()
  await pathEditor.fill('/discarded')
  await userEvent.keyboard('{Escape}')
  await expect
    .element(
      page.getByRole('button', { name: 'Navigate to Projects', exact: true }),
    )
    .toBeVisible()

  // Root navigation updates the controlled path and retains its caller styling.
  await page
    .getByRole('button', { name: 'Navigate to root', exact: true })
    .click()
  expect(
    getComputedStyle(
      page.getByRole('group', { name: 'File path', exact: true }).element(),
    ).marginTop,
  ).toBe('19px')
  await expect
    .element(
      page.getByRole('button', { name: 'Navigate to Projects', exact: true }),
    )
    .not.toBeInTheDocument()

  // Shared icons retain inherited color and the button's descendant dimensions.
  const action = page.getByRole('button', {
    name: 'Native action',
    exact: true,
  })
  await action.click()
  await expect
    .element(page.getByRole('status', { name: 'Native button actions' }))
    .toHaveTextContent('1')
  expect(getComputedStyle(action.element()).padding).toBe('19px')
  expect(getComputedStyle(action.element()).minWidth).toBe('123px')
  expect(
    getComputedStyle(action.element()).getPropertyValue('--native-button-test'),
  ).toBe('retained')

  // The public recipe composes directly on a native element too.
  expect(
    getComputedStyle(
      page
        .getByRole('button', { name: 'Recipe action', exact: true })
        .element(),
    ).height,
  ).toBe('32px')

  // Every compact control applies its icon dimensions at the SVG owner.
  for (const name of [
    'Native action',
    'Native dashboard action',
    'Native dropdown action',
  ]) {
    const icon = page
      .getByRole('button', { name, exact: true })
      .element()
      .querySelector('svg')!
    expect(getComputedStyle(icon).width).toBe('16px')
    expect(getComputedStyle(icon).height).toBe('16px')
    expect(getComputedStyle(icon).pointerEvents).toBe('none')
    expect(getComputedStyle(icon).color).toBe('rgb(17, 34, 51)')
  }

  // Defaults yield to caller styles, while a portaled icon keeps its size outside the control DOM.
  const defaultIcon = page.getByLabelText('Default native icon').element()
  expect(getComputedStyle(defaultIcon).width).toBe('20px')
  expect(getComputedStyle(defaultIcon).color).toBe('rgb(68, 85, 102)')

  // Caller styles replace the default dimensions and color, keeping native variables.
  const overriddenIcon = page.getByLabelText('Overridden native icon').element()
  expect(getComputedStyle(overriddenIcon).width).toBe('32px')
  expect(getComputedStyle(overriddenIcon).color).toBe('rgb(17, 34, 51)')
  expect(
    getComputedStyle(overriddenIcon).getPropertyValue('--native-icon-test'),
  ).toBe('retained')

  // A portal retains React context while leaving the control's DOM selector.
  const portalIcon = page.getByLabelText('Portaled native icon').element()
  expect(getComputedStyle(portalIcon).width).toBe('32px')
  expect(getComputedStyle(portalIcon).pointerEvents).toBe('auto')
  expect(getComputedStyle(portalIcon).color).toBe('rgb(17, 34, 51)')

  // Slot composition preserves the native link target.
  await expect
    .element(page.getByRole('link', { name: 'Native link', exact: true }))
    .toHaveAttribute('href', '#native-link')
})
