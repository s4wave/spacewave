---
version: alpha
name: Spacewave-design-system
description: "Spacewave is a dark, local-first application environment with a polished workbench feel: warm near-black surfaces, OKLCH brand reds, glass panels, compact tab chrome, dense object viewers, and a small amount of responsive motion. The app should feel precise and technical without becoming generic developer-tool gray. Marketing and onboarding surfaces may be more expressive, but they still share the same dark canvas, translucent cards, Manrope typography, Lucide icons, and brand-tinted interaction states."

colors:
  brand: "#ff9ea5"
  brand-highlight: "#ffa8ae"
  primary: "#de3955"
  primary-foreground: "#ffffff"
  violet: "#948ae3"
  link-hover: "{colors.violet}"
  accent: "#f63d6e"
  success: "#06ff8c"
  warning: "#fdba8c"
  error: "#ff8f8f"
  error-text: "#ffb3b3"
  error-bg: "#ff8f8f"
  error-border: "#ff8f8f"
  foreground: "#ffffff"
  foreground-alt: "#e1dddd"
  text-primary: "#f7f1ff"
  text-secondary: "#e8e3e3"
  text-muted: "#bcb5b5"
  background: "#1d1b1b"
  background-dark: "#0c0b0b"
  background-card: "#1a1818"
  background-card-alt: "#161414"
  background-landing: "#151313"
  background-get-started: "#181717"
  background-canvas: "#141212"
  background-primary: "#1c1a1a"
  background-secondary: "#211f1f"
  background-tertiary: "#2a2828"
  background-panel: "#262324"
  topbar-back: "#121212"
  shell-tab-active: "#2a2828"
  shell-tab-inactive: "#171616"
  shell-tab-text: "#bcb5b5"
  shell-tab-text-active: "#ffffff"
  border: "#464343"
  window-border: "#ffffff"
  foreground-border-subtle: "#ffffff"
  foreground-border: "#ffffff"
  foreground-border-strong: "#ffffff"
  tooltip-bg: "#1a1a1a"
  tooltip-text: "#e6e6e6"
  tooltip-border: "#ffffff"
  logo-base: "#030e31"
  logo-blue: "#37b1dc"
  logo-pink: "#c35aac"
  logo-purple: "#83298c"
  logo-dark: "#050b30"

typography:
  display-xl:
    fontFamily: "Manrope Variable, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif"
    fontSize: 48px
    fontWeight: 700
    lineHeight: 1.12
    letterSpacing: 0
  display-lg:
    fontFamily: "Manrope Variable, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif"
    fontSize: 36px
    fontWeight: 700
    lineHeight: 1.16
    letterSpacing: 0
  page-title:
    fontFamily: "Manrope Variable, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif"
    fontSize: 18px
    fontWeight: 600
    lineHeight: 1.35
    letterSpacing: 0
  panel-title:
    fontFamily: "Manrope Variable, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif"
    fontSize: 14px
    fontWeight: 600
    lineHeight: 1.4
    letterSpacing: 0
  section-title:
    fontFamily: "Manrope Variable, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif"
    fontSize: 12px
    fontWeight: 500
    lineHeight: 1.4
    letterSpacing: 0
  body-md:
    fontFamily: "Manrope Variable, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif"
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.6
    letterSpacing: 0
  body-sm:
    fontFamily: "Manrope Variable, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif"
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.5
    letterSpacing: 0
  control-sm:
    fontFamily: "Manrope Variable, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif"
    fontSize: 12px
    fontWeight: 500
    lineHeight: 1
    letterSpacing: 0
  micro-cap:
    fontFamily: "Manrope Variable, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif"
    fontSize: 10px
    fontWeight: 600
    lineHeight: 1.4
    letterSpacing: 0.08em
  metadata:
    fontFamily: "Manrope Variable, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif"
    fontSize: 9.6px
    fontWeight: 500
    lineHeight: 1.5
    letterSpacing: 0
  mono:
    fontFamily: "Pragmasevka, ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, Liberation Mono, Courier New, monospace"
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.5
    letterSpacing: 0

rounded:
  menu-button: 3px
  xs: 4px
  editor: 6px
  sm: 8px
  md: 12px
  lg: 16px
  xl: 20px
  pill: 9999px

spacing:
  xxs: 4px
  xs: 8px
  sm: 12px
  md: 16px
  lg: 24px
  xl: 32px
  xxl: 48px
  shell-header: 28px
  panel-header: 36px

components:
  app-canvas:
    backgroundColor: "{colors.background}"
    textColor: "{colors.foreground}"
    typography: "{typography.body-md}"
    rounded: 0px
    padding: 0px
  landing-canvas:
    backgroundColor: "{colors.background-landing}"
    textColor: "{colors.foreground}"
    typography: "{typography.body-md}"
    rounded: 0px
    padding: 0px
  glass-card:
    backgroundColor: "{colors.background-card}"
    textColor: "{colors.foreground}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.sm}"
    padding: 14px
  info-card:
    backgroundColor: "{colors.background-card}"
    textColor: "{colors.foreground}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.sm}"
    padding: 14px
  action-card:
    backgroundColor: "{colors.background-card}"
    textColor: "{colors.foreground}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.sm}"
    padding: 12px
  dashboard-button:
    backgroundColor: "{colors.background}"
    textColor: "{colors.foreground-alt}"
    typography: "{typography.control-sm}"
    rounded: "{rounded.menu-button}"
    padding: 8px 8px
  primary-action:
    backgroundColor: "{colors.background-card}"
    textColor: "{colors.brand}"
    typography: "{typography.control-sm}"
    rounded: "{rounded.xs}"
    padding: 8px 12px
  text-input:
    backgroundColor: "{colors.background}"
    textColor: "{colors.foreground}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 8px 12px
  shell-tab:
    backgroundColor: "{colors.shell-tab-inactive}"
    textColor: "{colors.shell-tab-text}"
    typography: "{typography.control-sm}"
    rounded: "{rounded.editor} {rounded.editor} 0 0"
    padding: 0px 8px
  panel-header:
    backgroundColor: "{colors.background-primary}"
    textColor: "{colors.foreground}"
    typography: "{typography.panel-title}"
    rounded: 0px
    padding: 0px 16px
  wizard-shell:
    backgroundColor: "{colors.background-card}"
    textColor: "{colors.foreground}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.md}"
    padding: 0px
  loading-card:
    backgroundColor: "{colors.background-card}"
    textColor: "{colors.foreground}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.sm}"
    padding: 14px
  badge:
    backgroundColor: "{colors.background-card-alt}"
    textColor: "{colors.foreground-alt}"
    typography: "{typography.micro-cap}"
    rounded: "{rounded.pill}"
    padding: 2px 8px
  tooltip:
    backgroundColor: "{colors.tooltip-bg}"
    textColor: "{colors.tooltip-text}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 6px 8px
  primary-hover:
    backgroundColor: "{colors.background-card}"
    textColor: "{colors.brand-highlight}"
    typography: "{typography.control-sm}"
    rounded: "{rounded.xs}"
    padding: 8px 12px
  primary-drag-affordance:
    backgroundColor: "{colors.background-dark}"
    textColor: "{colors.primary}"
    typography: "{typography.control-sm}"
    rounded: "{rounded.xs}"
    padding: 4px 8px
  primary-foreground-reference:
    backgroundColor: "{colors.background-dark}"
    textColor: "{colors.primary-foreground}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 4px 8px
  link-hover:
    backgroundColor: "{colors.background}"
    textColor: "{colors.link-hover}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 0px
  accent-tint:
    backgroundColor: "{colors.background-dark}"
    textColor: "{colors.accent}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 4px 8px
  status-success:
    backgroundColor: "{colors.success}"
    textColor: "{colors.background-dark}"
    typography: "{typography.micro-cap}"
    rounded: "{rounded.pill}"
    padding: 2px 8px
  status-warning:
    backgroundColor: "{colors.warning}"
    textColor: "{colors.background-dark}"
    typography: "{typography.micro-cap}"
    rounded: "{rounded.pill}"
    padding: 2px 8px
  destructive-callout:
    backgroundColor: "{colors.background-dark}"
    textColor: "{colors.error-text}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 8px 12px
  destructive-bg-reference:
    backgroundColor: "{colors.background-dark}"
    textColor: "{colors.error-bg}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 4px 8px
  destructive-border-reference:
    backgroundColor: "{colors.background-dark}"
    textColor: "{colors.error-border}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 4px 8px
  text-primary-reference:
    backgroundColor: "{colors.background}"
    textColor: "{colors.text-primary}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 0px
  text-secondary-reference:
    backgroundColor: "{colors.background}"
    textColor: "{colors.text-secondary}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 0px
  text-muted-reference:
    backgroundColor: "{colors.background}"
    textColor: "{colors.text-muted}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 0px
  onboarding-card:
    backgroundColor: "{colors.background-get-started}"
    textColor: "{colors.foreground}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.sm}"
    padding: 16px
  destructive-status:
    backgroundColor: "{colors.background-dark}"
    textColor: "{colors.error}"
    typography: "{typography.micro-cap}"
    rounded: "{rounded.pill}"
    padding: 2px 8px
  canvas-panel:
    backgroundColor: "{colors.background-canvas}"
    textColor: "{colors.foreground}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.sm}"
    padding: 12px
  secondary-panel:
    backgroundColor: "{colors.background-secondary}"
    textColor: "{colors.foreground}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.sm}"
    padding: 12px
  tertiary-surface:
    backgroundColor: "{colors.background-tertiary}"
    textColor: "{colors.foreground}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.sm}"
    padding: 12px
  editor-panel:
    backgroundColor: "{colors.background-panel}"
    textColor: "{colors.foreground}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.editor}"
    padding: 8px
  topbar-shell:
    backgroundColor: "{colors.topbar-back}"
    textColor: "{colors.foreground}"
    typography: "{typography.control-sm}"
    rounded: 0px
    padding: 0px 8px
  shell-tab-active:
    backgroundColor: "{colors.shell-tab-active}"
    textColor: "{colors.shell-tab-text-active}"
    typography: "{typography.control-sm}"
    rounded: "{rounded.editor} {rounded.editor} 0 0"
    padding: 0px 8px
  border-reference:
    backgroundColor: "{colors.border}"
    textColor: "{colors.foreground}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 4px 8px
  window-border-reference:
    backgroundColor: "{colors.window-border}"
    textColor: "{colors.background}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 4px 8px
  foreground-border-subtle-reference:
    backgroundColor: "{colors.foreground-border-subtle}"
    textColor: "{colors.background}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 4px 8px
  foreground-border-reference:
    backgroundColor: "{colors.foreground-border}"
    textColor: "{colors.background}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 4px 8px
  foreground-border-strong-reference:
    backgroundColor: "{colors.foreground-border-strong}"
    textColor: "{colors.background}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 4px 8px
  tooltip-border-reference:
    backgroundColor: "{colors.tooltip-border}"
    textColor: "{colors.tooltip-bg}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 4px 8px
  logo-base-reference:
    backgroundColor: "{colors.logo-base}"
    textColor: "{colors.logo-blue}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xl}"
    padding: 8px
  logo-pink-reference:
    backgroundColor: "{colors.logo-pink}"
    textColor: "{colors.logo-dark}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xl}"
    padding: 8px
  logo-purple-reference:
    backgroundColor: "{colors.logo-purple}"
    textColor: "{colors.foreground}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xl}"
    padding: 8px
---

## Overview

Spacewave's visual language is a dense dark workbench, not a light SaaS shell
and not a black marketing poster. The app lives on warmed near-black OKLCH
surfaces with white foreground text, soft secondary text, and a red-pink brand
accent used sparingly for focus, selected state, and primary action. The
strongest visual identity comes from glass cards, compact tab chrome, a
gradient animated app icon, and precise control density.

The system has two related modes. The app mode is compact: panels, ObjectViewer
surfaces, Space layouts, session settings, command palettes, and wizards should
optimize for repeated work and scanning. The marketing and onboarding mode is
more expressive: taller sections, scroll reveal, larger titles, bigger cards,
and animated logo treatments are allowed, but the same tokens and interaction
rules still apply.

**Key Characteristics:**
- Warm near-black surfaces instead of neutral slate: `{colors.background}`,
  `{colors.background-card}`, and `{colors.background-primary}` define the base.
- Brand red-pink as an interaction accent, not a full-page wash. Use
  `{colors.brand}` mostly at 5-15% background opacity or 30-60% border opacity.
- Glassmorphism stack: translucent card background, subtle white border,
  `backdrop-blur-sm`, and no heavy shadow except menus, toasts, and modal-like
  overlays.
- Compact app chrome: 28px shell header, 36px detail-panel header, 22px shell
  tabs, 7px-9px icon buttons, and 12px section labels.
- Manrope Variable for UI and display; Pragmasevka only for code, terminals,
  hashes, object refs, and numeric technical output.
- Lucide-style line icons from `react-icons/lu` for UI meaning. Icons should be
  small, aligned, and inside tinted square or circular containers when they
  carry row hierarchy.
- Motion is functional: hover lifts, reveal fades, loading shine borders, logo
  parallax, and progress animation. Respect reduced motion.

## Colors

> **Source surfaces:** `web/style/app.css`, `app/landing/*`,
> `app/session/dashboard/*`, `app/wizard/WizardShell.tsx`,
> `web/ui/loading/*`, and `web/style/flexlayout/*`.

### Brand

- **Brand** (`{colors.brand}`): the primary Spacewave accent. It reads as a
  warm red-pink in SDR and becomes richer on P3/HDR displays.
- **Brand Highlight** (`{colors.brand-highlight}`): hover and animated accent
  color. Use it for hover borders, animated logo gradients, and active edge
  accents.
- **Primary** (`{colors.primary}`): stronger accent used by lower-level
  FlexLayout and shell drag affordances. Prefer `{colors.brand}` for product UI
  unless the existing shell styling already uses primary.
- **Violet / Link Hover** (`{colors.violet}`): hover treatment for links.
  It is a supporting link color, not a second brand.

### Surface

- **Background** (`{colors.background}`): default app canvas.
- **Background Dark** (`{colors.background-dark}`): deepest surface, used for
  code blocks, strong recesses, and boot mockup interiors.
- **Background Card** (`{colors.background-card}`): standard card base. It is
  usually rendered at 20-60% opacity, not as a fully opaque slab.
- **Background Card Alt** (`{colors.background-card-alt}`): dense table or
  comparison surface.
- **Background Landing** (`{colors.background-landing}`): landing and session
  dashboard canvas.
- **Background Get Started** (`{colors.background-get-started}`): auth and
  onboarding card shell.
- **Background Primary / Secondary / Panel**: editor, FlexLayout, and panel
  system surfaces. Use these in shell/object-viewer chrome instead of inventing
  new dark grays.

### Text

- **Foreground** (`{colors.foreground}`): primary text and selected shell
  labels.
- **Foreground Alt** (`{colors.foreground-alt}`): secondary text. Use opacity
  modifiers heavily: `/70` for useful detail, `/50` for metadata, `/40` for
  empty state, `/30` for very low-emphasis icons.
- **Text Primary / Secondary / Muted**: legacy and shell-facing text tokens
  still used in editor and FlexLayout surfaces. New app code should prefer
  `foreground` and `foreground-alt`.

### Semantic

- **Success** (`{colors.success}`): confirmation, completed step, positive
  status.
- **Warning** (`{colors.warning}`): caution, expiring or degraded state.
- **Error** (`{colors.error}`): destructive and failed states. Use low-opacity
  backgrounds such as `bg-destructive/5` or error token equivalents before
  reaching for solid fills.

### Opacity Rules

The design depends on opacity, not new colors. These are the common steps:

| Use | Token Pattern |
|---|---|
| Subtle border | `border-foreground/6` |
| Standard header divider | `border-foreground/8` |
| Interactive border | `border-foreground/10` |
| Hover border | `border-foreground/12` or `/15` |
| Strong selected border | `border-brand/30` |
| Standard card | `bg-background-card/30` |
| Hover card | `bg-background-card/50` or `/60` |
| Brand tint | `bg-brand/10`, hover `bg-brand/15` |
| Muted text | `text-foreground-alt/50` |
| Disabled or empty | `text-foreground-alt/40` |

## Typography

### Font Family

Spacewave uses **Manrope Variable** as the default display and UI family. It is
geometric enough to feel technical, but softer than a monospace-heavy developer
tool. Use `font-display`, `font-heading`, or the default font stack for all
ordinary interface text.

Use **Pragmasevka** only for code, terminal output, object refs, hashes, diffs,
numeric debug values, and other literal technical data. Do not use monospace as
a brand decoration.

### Hierarchy

| Token | Size | Weight | Line Height | Use |
|---|---:|---:|---:|---|
| `{typography.display-xl}` | 48px | 700 | 1.12 | Large landing title only |
| `{typography.display-lg}` | 36px | 700 | 1.16 | Landing section title |
| `{typography.page-title}` | 18px | 600 | 1.35 | Focused page title |
| `{typography.panel-title}` | 14px | 600 | 1.4 | Panel header title |
| `{typography.section-title}` | 12px | 500 | 1.4 | Section heading |
| `{typography.body-md}` | 14px | 400 | 1.6 | Body copy and descriptions |
| `{typography.body-sm}` | 12px | 400 | 1.5 | Dense UI body |
| `{typography.control-sm}` | 12px | 500 | 1 | Buttons and compact controls |
| `{typography.micro-cap}` | 10px | 600 | 1.4 | Step labels, badges, nav microcopy |
| `{typography.mono}` | 12px | 400 | 1.5 | Code, refs, terminal, hashes |

### Principles

- App panels should stay small. Use `text-sm` for panel titles, `text-xs` for
  section labels, and `text-[0.6rem]` for metadata.
- Marketing and onboarding may use `text-3xl`, `text-4xl`, and `text-2xl`, but
  app chrome should not inherit hero-scale type.
- Use uppercase only for micro labels, badges, and small nav items. Do not make
  normal panel headings uppercase.
- Keep letter spacing at 0 for normal text. Use `tracking-widest` only on
  micro labels such as wizard steps and group labels.
- Use `select-none` on headings, labels, tabs, badges, and non-editable chrome.

## Layout

### App Shell

The app shell is a full-height workbench:

- Canvas: `{components.app-canvas}`.
- Electron top bar: 28px when present.
- Shell tab header: `--spacing-shell-header` 28px.
- Shell tabs: 22px tall, compact, clipped to 100px max width, rounded only on
  top corners.
- Split workspaces use FlexLayout. Active tabsets get stronger borders and HDR
  glow where available; inactive tabsets recede.
- Nested ObjectViewer layouts must not inherit shell-level tab styles.

### Panels

Detail panels follow a consistent structure:

```text
+-----------------------------------------------+
| 36px header: icon, title, metadata, actions    |
+-----------------------------------------------+
| scrollable content: px-4 py-3                  |
| sections in priority order                     |
| danger zone last                               |
+-----------------------------------------------+
```

Rules:

- Header uses `h-9`, `border-b`, `border-foreground/8`, and `px-4`.
- Title uses `text-sm font-semibold tracking-tight`.
- Body uses `flex-1 overflow-auto px-4 py-3`.
- Section groups use `space-y-3`.
- Section heading uses `text-xs font-medium`, icon `h-3.5 w-3.5`, and
  `gap-1.5`.
- Do not show empty placeholder sections above sections with real content.

### Focused Pages

Use focused centered pages for route-level account, billing, setup, or recovery
flows that have one narrow task:

- Root: `relative flex h-full w-full items-start justify-center pt-16`.
- Content: `w-full max-w-md px-4`.
- Page title row: icon `h-5 w-5`, title `text-lg font-semibold
  tracking-tight`, actions beside it.
- Major groups: `space-y-6`, not a dense stack of unrelated cards.

### Landing And Onboarding

Landing surfaces are allowed to be more spacious, but still must feel like
Spacewave:

- Root canvas: `{colors.background-landing}`.
- Hero: animated logo, `[SPACEWAVE]` wordmark, bracket-like technical
  navigation, centered quickstart flow, and short local-first copy.
- Sections: `px-4 py-20`, `max-w-5xl`, optional one-pixel separator.
- Section labels: uppercase `text-xs font-semibold tracking-widest`.
- Section titles: `text-3xl @lg:text-4xl font-semibold` or `font-bold`.
- Cards: glass cards with `border-foreground/6 bg-background-card/30`.
- Scroll reveal: fade + translate from `translate-y-8 opacity-0` to visible.

## Elevation And Depth

Spacewave uses translucency, borders, and subtle motion more than shadows.

| Level | Treatment | Use |
|---|---|---|
| 0 | Flat near-black surface | App canvas, editor interiors |
| 1 | `bg-background-card/30`, `border-foreground/6` | Info cards, wizards, object panels |
| 2 | Glass card with `backdrop-blur-sm` and hover border | Action cards, landing cards, popover-like panels |
| 3 | Menu shadow plus opaque card | Menus, dropdowns, toasts, overlays |
| 4 | Shine border / animated logo glow | Boot and brand moments only |

Avoid heavy drop shadows for ordinary cards. Use `shadow-menu` for menus and
toasts, and use subtle `shadow-lg` only for modal or login-style overlays.

### Motion

- Buttons and cards: `transition-all duration-150` for app UI.
- Landing cards and reveal: `duration-300` to `duration-700`.
- Hover lift is small: `hover:-translate-y-0.5` or `-translate-y-[1px]`.
- Loading shine border and animated logo should stop or reduce when reduced
  motion is active.
- Do not use looping motion to attract attention in ordinary panels.

## Shapes

| Token | Value | Use |
|---|---:|---|
| `{rounded.menu-button}` | 3px | Dense dashboard buttons |
| `{rounded.xs}` | 4px | Compact inputs and small inline surfaces |
| `{rounded.editor}` | 6px | Shell tabs and editor panel corners |
| `{rounded.sm}` | 8px | Standard cards |
| `{rounded.md}` | 12px | Wizard shell and route cards |
| `{rounded.lg}` | 16px | Logo-adjacent large shells |
| `{rounded.xl}` | 20px | Animated logo image corners |
| `{rounded.pill}` | 9999px | Badges, pills, progress chips |

Do not round dense app controls into large pills. Pills are for badges, status
chips, floating bars, and compact overlays.

## Components

### App Canvas

**`app-canvas`** is the default full-height app surface.

- Background `{colors.background}`.
- Text `{colors.foreground}`.
- No outer padding.
- Child surfaces own their spacing and scrolling.

### Landing Canvas

**`landing-canvas`** is used for public, onboarding, and empty-session surfaces.

- Background `{colors.background-landing}`.
- Allows centered logo/quickstart composition.
- May use larger cards and reveal motion.
- Must not switch to a light marketing theme.

### Info Card

**`info-card`** is the standard static content card.

- `border-foreground/6 bg-background-card/30 rounded-lg border p-3.5`.
- Use for identifiers, settings summaries, status groups, and non-interactive
  details.
- Do not wrap a component that already renders its own bordered card.

### Action Card

**`action-card`** is a clickable row/card with an icon box, label, and
description.

- Default: `border-foreground/6 bg-background-card/30`.
- Hover: `hover:border-foreground/12 hover:bg-background-card/50`.
- Icon box usually `bg-foreground/5` or `bg-brand/10`.
- Destructive variant uses low-opacity error/destructive border and background.

### Stat Card

**`stat-card`** displays a metric with a brand icon box.

- `border-foreground/6 bg-background-card/30 rounded-lg border p-3`.
- Hover: `hover:border-foreground/12 hover:bg-background-card/60`.
- Icon container: `size-9 bg-brand/10`, icon `size-4.5 text-brand`.
- Label `text-xs text-foreground-alt`; value `text-sm font-medium`.

### Dashboard Button

**`dashboard-button`** is dense panel chrome.

- `h-7 px-2 rounded-menu-button`.
- Border `border-foreground/8`; transparent background.
- Hover `bg-foreground/5 border-foreground/15 text-foreground`.
- Text `text-xs`; icons `h-3.5 w-3.5` or `h-4 w-4`.
- Wrap action buttons in tooltips when icon-only or when the action is not
  self-evident.

### Primary Action

**`primary-action`** is the app's brand-tinted confirm button.

- Border `border-brand/30`.
- Background `bg-brand/10`.
- Hover `border-brand/50 bg-brand/15`.
- Text `text-foreground text-xs`.
- Height usually `h-7` in wizards and `h-9` on focused pages.
- Do not use a solid brand fill for ordinary app actions.

### Text Input

**`text-input`** is the polished input style for forms and wizards.

- `border-foreground/10 bg-background/20 text-foreground`.
- Placeholder `text-foreground-alt/40`.
- Height `h-9`.
- Focus `focus-visible:border-brand/50 focus-visible:ring-brand/15`.
- Use `font-mono` only when the input is a token, URL, hash, path, or object
  reference.

### Radio Option

**`radio-option`** is for two-to-four mutually exclusive choices.

- Base `rounded-md border p-2.5 text-left`.
- Unselected `border-foreground/10 bg-background/20`.
- Hover `hover:border-foreground/20`.
- Selected `border-brand/30 bg-brand/5`.
- Use the built-in radio dot and optional icon/tag. Do not use `ring-primary`
  for selection.

### Wizard Shell

**`wizard-shell`** is the persistent object-creation shell.

- Outer: centered, `px-4 py-10`, scrollable.
- Card: `max-w-lg rounded-xl border-foreground/6 bg-background-card/30
  backdrop-blur-sm`.
- Header: `h-9 border-b border-foreground/8 px-4`.
- Step label: `text-foreground-alt/50 text-[0.6rem] font-medium
  tracking-widest uppercase`.
- Body: `px-4 py-3 space-y-3`.
- Footer: `border-t border-foreground/8 px-4 py-3`.
- Long-running work becomes a progress step with watched status, not a stuck
  button.

### Loading Screen

**`loading-screen`** owns route-level and boot loading.

- Full viewport centered column on `{colors.background}`.
- Optional animated logo slot.
- Optional shine border using logo blue, pink, purple, and blue.
- Title `text-2xl font-semibold tracking-tight`.
- Detail `text-sm text-foreground-alt/70`.
- Progress bar width `w-64`.
- Retry and Back actions use compact icon buttons.

### Loading Card

**`loading-card`** owns bounded panel/object loading.

- Uses the shared `LoadingView` contract: `loading`, `active`, `synced`, or
  `error`.
- Optional progress, rate, activity, retry, and cancel.
- Domain wrappers adapt watches/resources to `LoadingView`; primitives stay
  presentational.
- Use render delay for list, grid, and chat loads so sub-300ms waits do not
  flash loading UI.

### Shell Tab

**`shell-tab`** is compact top workbench chrome.

- Height 22px in shell layout; 20px in the top-bar-style strip.
- Max width 100px, ellipsized text, `font-size: 11px`.
- Top corners only, no underline pseudo-element.
- Inactive tabs are translucent and blurred; selected tabs are opaque and
  brighter.
- Active grid tabsets may receive stronger border or HDR glow.

### Tooltip

**`tooltip`** is black glass popover chrome.

- Background `bg-black/90` or `{colors.tooltip-bg}`.
- Border `border-foreground/20`.
- Text white / foreground.
- Use for icon-only actions, technical labels, truncated refs, and row
  explanations.

## Iconography

Use `react-icons/lu` Lucide icons for UI controls and sections. Keep icon sizes
stable:

| Size | Use |
|---|---|
| `h-2.5 w-2.5` | Tiny badges |
| `h-3 w-3` | Secondary row glyphs |
| `h-3.5 w-3.5` | Section headings and dense buttons |
| `h-4 w-4` | Panel header actions and primary row icons |
| `h-4.5 w-4.5` | Stat-card icons |
| `h-5 w-5` | Focused page leading icons |

Icon containers:

- `size-5 bg-foreground/5` for secondary rows.
- `size-7 bg-foreground/5` for compact primary rows.
- `size-8 bg-brand/10` for primary actions and status rows.
- `size-9 bg-brand/10` for metric/stat cards.

Do not use standalone decorative icons as filler. An icon should clarify a
section, action, status, or data type.

## Responsive Behavior

Spacewave uses container queries and viewport-height variants heavily.

### Breakpoints And Variants

| Variant | Trigger | Use |
|---|---|---|
| `narrow` | max-width 640px | Collapse dense landing/onboarding surfaces |
| `wide` | min-width 1920px | Give large displays more room |
| `ultrawide` | min-width 2560px | Avoid over-stretched content |
| `tall` | min-height 800px | Center landing hero vertically |
| `short` | max-height 580px | Hide or compress footer adornments |
| `very-short` | max-height 470px | Hide logo when necessary |
| `ultra-short` | max-height 390px | Hide hero wordmark when necessary |
| `@lg` | container min-width 32rem | Landing and component scale-up |
| `@2xl` | container min-width 42rem | Larger landing spacing |

### Rules

- App panels must work inside split panes; design for container width, not only
  viewport width.
- Stable chrome dimensions prevent layout shift: fixed tab heights, fixed
  header heights, fixed icon boxes, fixed progress widths.
- Landing hides or compresses decorative content before controls overlap.
- If a surface is too narrow to be usable, show a minimal width gate rather than
  allowing broken controls.
- Text inside buttons and tabs must truncate or wrap intentionally. Do not let
  labels overflow their containers.

## Do's And Don'ts

### Do

- Use the existing OKLCH theme tokens in `web/style/app.css`.
- Use opacity modifiers before creating new color tokens.
- Use `InfoCard`, `DashboardButton`, `RadioOption`, loading primitives, and
  wizard shell patterns before creating a local one-off.
- Keep app chrome compact, with panel headers at `h-9` and controls at `h-7`
  or `h-9`.
- Put identifiers, object refs, hashes, paths, and code in Pragmasevka mono.
- Put destructive actions last, with low-opacity destructive tint and clear
  copy.
- Use watched or streaming state for mutable backend progress and loading.
- Respect reduced motion for animated logo, shine borders, and loading effects.

### Don't

- Do not make app panels look like landing-page hero sections.
- Do not introduce a second accent palette for a local feature.
- Do not use light cards or white canvases for normal Spacewave app surfaces.
- Do not use `bg-muted`, `ring-primary`, deprecated `text-text-*` tokens, or
  local gray ramps in new UI.
- Do not stack cards inside cards unless the inner element is a real repeated
  item, modal, or bounded tool.
- Do not center large empty states in dense panels; use compact muted rows.
- Do not use polling loops for progress or status just to drive UI.
- Do not persist wizard state on every keystroke.
- Do not hide long-running setup behind one blocking action with no progress.

## Iteration Guide

1. Start with the surface owner: shell, panel, focused page, wizard, loading,
   landing, or object viewer.
2. Use the component token names directly: `{colors.background-card}`,
   `{colors.brand}`, `{components.info-card}`, `{components.dashboard-button}`.
3. Preserve density in app surfaces. Use larger spacing only on landing,
   onboarding, and route-level focused flows.
4. Choose one interaction state per change: default, hover, selected, loading,
   empty, error, disabled.
5. For mutable state, wire a watched/streaming state owner first, then render
   with the existing loading or status primitives.
6. Verify app code changes with `bun run tsgo --noEmit` and focused tests for
   the changed surface.
7. When editing this design document, keep it current-state only. Remove stale
   compatibility notes rather than documenting old and new systems side by side.

## References

- `web/style/app.css`: Tailwind v4 theme variables, fonts, HDR/P3 variants,
  shell styles, scrollbars, and hero button class.
- `web/ui/InfoCard.tsx`: standard static card.
- `web/ui/DashboardButton.tsx`: compact panel action button.
- `web/ui/RadioOption.tsx`: selected/unselected choice row.
- `web/ui/StatCard.tsx`: metric card with brand icon box.
- `web/ui/loading/*`: `LoadingView`, `Spinner`, `ProgressBar`,
  `LoadingInline`, `LoadingCard`, and `LoadingScreen`.
- `app/wizard/WizardShell.tsx`: persistent wizard shell.
- `app/session/dashboard/SessionDashboard.tsx`: empty-session dashboard and
  command-palette onboarding surface.
- `app/landing/*`: landing page hero, section, CTA, reveal, legal, and
  community surfaces.
- `app/ShellFlexLayout.tsx` and `web/style/flexlayout/*`: workbench tab and
  split-pane behavior.
