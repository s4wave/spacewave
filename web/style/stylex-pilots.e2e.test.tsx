import { act, useState } from 'react'
import { createRoot } from 'react-dom/client'
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
import { Button, buttonVariants } from '../ui/button.js'
import { DashboardButton } from '../ui/DashboardButton.js'
import { DropdownTriggerButton } from '../ui/DropdownTriggerButton.js'
import { IconContext, LuPlus } from '../images/icons.js'
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
  CollapsibleSection,
  collapsibleSectionActionStyles,
} from '../ui/CollapsibleSection.js'

const styles = stylex.create({
  override: { padding: 19, borderRadius: 9 },
  headerOverride: { paddingBlock: 19, paddingInline: 19, borderRadius: 9 },
  firstRule: { color: 'rgb(17, 34, 51)' },
  secondRule: { color: 'rgb(68, 85, 102)' },
  largeIcon: { width: 32, height: 32 },
  minimumWidth: (minWidth: number) => ({ minWidth }),
})

// PilotJourney exercises shared composition together with real app and plugin consumers.
function PilotJourney() {
  // Keep the fixture's independent interactions local to their mounted controls.
  const [selected, setSelected] = useState('None')
  const [radioSelected, setRadioSelected] = useState(false)
  const [sectionOpen, setSectionOpen] = useState(false)
  const [actions, setActions] = useState(0)
  const [sheetOpen, setSheetOpen] = useState(false)
  const [buttonActions, setButtonActions] = useState(0)

  return (
    <main>
      <IconContext.Provider value={{ color: 'rgb(17, 34, 51)' }}>
        <Button
          sx={[styles.headerOverride, styles.minimumWidth(123)]}
          style={{ '--native-button-test': 'retained' }}
          onClick={() => setButtonActions((value) => value + 1)}
        >
          <LuPlus {...stylex.props(styles.largeIcon)} />
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
      <button
        type="button"
        sx={buttonVariants({ variant: 'outline', size: 'sm' })}
      >
        Recipe action
      </button>
      <output aria-label="Native button actions">{buttonActions}</output>
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
      <Window data-area-id="fixture-window">
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
  const card = document.querySelector('main > div')!
  const style = getComputedStyle(card)
  expect(style.padding).toBe('19px')
  expect(style.borderRadius).toBe('9px')

  // A plugin interaction retains the same shared card and its caller's final override.
  await page.getByRole('button', { name: 'native-css', exact: true }).click()
  await expect
    .poll(
      () =>
        document.querySelector('output[aria-label="Selected tag"]')
          ?.textContent,
    )
    .toBe('native-css')
  expect(document.querySelector('main > div')).toBe(card)
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

  // Slot composition preserves the native link target.
  await expect
    .element(page.getByRole('link', { name: 'Native link', exact: true }))
    .toHaveAttribute('href', '#native-link')
})
