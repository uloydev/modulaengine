# ModulaEngine Admin UI & Visual Design Guidelines

**Status**: Production Standard  
**Scope**: Svelte 5 Admin SPA (`web/`) and embedded console interfaces  

---

## 1. Visual Anti-Slop Hard Gates

The ModulaEngine Admin UI must feel like a precision developer tool (Linear, Supabase Studio, Datadog), not an AI-generated landing page template. The following rules are non-negotiable.

### 1.1 Forbidden AI Visual Patterns

| Pattern | Rule | Rationale |
|---|---|---|
| **Purple/Blue Gradients** | **FORBIDDEN** (R-01) | Over-represented AI template default. Use solid neutral surfaces with high-contrast text. |
| **Universal Glassmorphism** | **FORBIDDEN** (R-10) | Backdrop blur flattens visual hierarchy. Max 1-2 elements (e.g. sticky table header). |
| **Universal Pill Shapes** | **FORBIDDEN** (R-11) | Buttons, badges, and inputs must use consistent, subtle radii (`rounded-md` / 4–6px). |
| **Full-Page Floating Shadows** | **FORBIDDEN** (R-12) | Soft floating shadows look muddy. Use subtle 1px borders (`border-neutral-200 / border-neutral-800`). |
| **Glow Everywhere** | **FORBIDDEN** (R-13) | Glow on cards, borders, and buttons creates visual noise. Use matte surfaces. |
| **Fake Bento Grids & Terminals** | **FORBIDDEN** (R-05) | Layouts must reflect actual data density and workflows, not decorative mosaic tiles. |
| **Em Dash (`—`) in UI Copy** | **FORBIDDEN** (R-02) | Use comma, colon, or parentheses. UI copy must be natural and terse. |
| **AI Buzzwords** | **FORBIDDEN** (R-16) | Never use "AI-powered", "revolutionary", "seamless", "next-gen". State clear technical facts. |

---

## 2. Design System & Visual Tokens

### 2.1 Color Hunt Palette Specifications & Token Mappings

ModulaEngine adopts curated, high-contrast palettes from [Color Hunt](https://colorhunt.co) adhering strictly to Rule R-29 (2-3 base tones + 1 deliberate accent) and WCAG AA contrast standards.

#### Dark Mode Primary (Default Admin Console)
Source: [Color Hunt Palette #222831393e4600adb5eeeeee](https://colorhunt.co/palette/222831393e4600adb5eeeeee)

| Token Name | Hex Code | Role & Usage | Contrast vs Canvas | Contrast vs Surface | WCAG Compliance |
|---|---|---|---|---|---|
| `--bg-canvas` | `#222831` | Deep base canvas, full app background, viewport margins | Base (1:1) | — | Base Canvas |
| `--bg-surface` | `#393E46` | Data grid cards, navigation sidebar, modal backgrounds | 1.4:1 | Base (1:1) | Elevation Layer |
| `--border-subtle` | `#4B5360` | 1px data cell dividers, subtle table borders, input outlines | 1.8:1 | 1.4:1 | Visual Structure |
| `--accent-primary` | `#00ADB5` | Primary action button, active tab indicator, selected row focus | 5.4:1 | 3.9:1 | UI Component (3:1+) |
| `--text-primary` | `#EEEEEE` | High-contrast body text, column headers, active labels | 12.8:1 | 9.3:1 | WCAG AAA (>7:1) |
| `--text-muted` | `#CBD5E1` | Secondary labels, row counts, timestamp metadata | 10.0:1 | 7.3:1 | WCAG AAA (>7:1) |

> **Critical Accessibility Guardrail for Buttons**:
> Text rendered inside primary accent buttons (`bg-[#00ADB5]`) **must use dark text `#222831`** (5.40:1 contrast, passes WCAG AA). White text (`#FFFFFF`) on `#00ADB5` yields only 2.75:1 and is **strictly prohibited**.

#### Light Mode Alternative (High-Legibility Workspace)
Source: [Color Hunt Palette #f9f7f7dbe2ef3f72af112d4e](https://colorhunt.co/palette/f9f7f7dbe2ef3f72af112d4e)

| Token Name | Hex Code | Role & Usage | Contrast vs Canvas | Contrast vs Surface | WCAG Compliance |
|---|---|---|---|---|---|
| `--bg-canvas-light` | `#F9F7F7` | Clean white/light-grey canvas | Base (1:1) | — | Base Canvas |
| `--bg-surface-light` | `#FFFFFF` | Card containers, data grid viewport, dropdown panels | 1.1:1 | Base (1:1) | Elevation Layer |
| `--border-light` | `#DBE2EF` | Subdued cell dividers, container strokes | 1.4:1 | 1.3:1 | Visual Structure |
| `--accent-light` | `#3F72AF` | Primary button CTA, link hover, focus ring | 4.8:1 | 4.4:1 | WCAG AA (>4.5:1) |
| `--text-primary-light` | `#112D4E` | Deep navy high-contrast text and headers | 14.2:1 | 15.6:1 | WCAG AAA (>7:1) |
| `--text-muted-light` | `#4B5563` | Subtitle text, muted metadata, input placeholders | 7.1:1 | 7.8:1 | WCAG AAA (>7:1) |

#### Semantic Status Signals (Dark & Light Verified)
- **Success**: `#10B981` (Emerald-500) — Contrast: 5.85:1 vs Canvas `#222831` (WCAG AA Pass)
- **Warning**: `#F59E0B` (Amber-500) — Contrast: 6.91:1 vs Canvas `#222831` (WCAG AA Pass)
- **Danger / Error**: `#F87171` (Rose-400) — Contrast: 5.36:1 vs Canvas `#222831` (WCAG AA Pass)
- **Info**: `#38BDF8` (Sky-400) — Contrast: 8.78:1 vs Canvas `#222831` (WCAG AAA Pass)

### 2.2 CSS Variables Specification (`app.css`)
```css
:root {
  /* Light Mode Default */
  --bg-canvas: #F9F7F7;
  --bg-surface: #FFFFFF;
  --border-subtle: #DBE2EF;
  --accent-primary: #3F72AF;
  --accent-text: #FFFFFF;
  --text-primary: #112D4E;
  --text-muted: #4B5563;
}

.dark {
  /* Dark Mode Primary */
  --bg-canvas: #222831;
  --bg-surface: #393E46;
  --border-subtle: #4B5360;
  --accent-primary: #00ADB5;
  --accent-text: #222831; /* Dark text on teal button for 5.4:1 contrast */
  --text-primary: #EEEEEE;
  --text-muted: #CBD5E1;   /* High-contrast muted text for 7.3:1 contrast */
}
```

### 2.3 Typography & Density
- **UI Font**: Clean system sans-serif (`Inter`, system UI font stack).
- **Tabular & Code Font**: Monospace (`JetBrains Mono`, `Geist Mono`) used strictly for UUIDs, BIGINT IDs, JSON editors, and raw timestamps.
- **Information Density**: Compact enterprise rows (table row height 32–36px), readable 13px/14px body text, clear column dividers.
- **Data Grid Readability (Dark Mode)**:
  - Table headers: `bg-[#222831] text-[#CBD5E1] text-xs uppercase tracking-wider font-semibold border-b border-[#4B5360]`.
  - Data rows: `bg-[#393E46] text-[#EEEEEE] text-sm border-b border-[#4B5360]/60 hover:bg-[#434A54] transition-colors`.
  - Selected row: `bg-[#00ADB5]/15 border-l-2 border-l-[#00ADB5]`.
  - Focus cell: `ring-1 ring-inset ring-[#00ADB5] outline-none`.

### 2.4 Dual-Theme Completeness & Verification (R-34)
Both Light and Dark modes are first-class:
- Dark mode must maintain crisp contrast without muddy grey-on-grey text (WCAG AA 4.5:1 minimum).
- Switching themes must instantly update all surfaces, borders, and syntax highlighters without flashes or broken styles.
- Verification command: Validate all color pairings using the mathematical WCAG 2.x relative luminance formula or `contrast-check.py`. Any pairing below 4.5:1 for body copy or 3.0:1 for large text / components is rejected.

---

## 3. Svelte 5 Architecture & Best Practices

All frontend code in `web/` is written in modern **Svelte 5**:

### 3.1 Svelte 5 Runes Invariant
Never use deprecated Svelte 3/4 paradigms (`let:`, `export let`, writable stores `$store`):
```svelte
<script lang="ts">
  // Good: Modern Svelte 5 Runes
  interface Props {
    collectionName: string;
    onRecordSelect?: (id: string) => void;
  }

  let { collectionName, onRecordSelect }: Props = $props();
  let search = $state('');
  let filteredCount = $derived(records.filter(r => r.matches(search)).length);

  $effect(() => {
    // Synchronize or handle lifecycle with explicit cleanup
    return () => { /* cleanup */ };
  });
</script>
```

### 3.2 Virtualized Data Grid Standards
- Collections exceeding 100 rows must render via virtual scrolling (DOM nodes limited to visible viewport + 5 buffer rows).
- Column sorting, multi-column filtering, and inline cell editing must execute in sub-16ms frames (60fps).
- Cells must format dates to UTC or browser locale cleanly without raw epoch timestamps.

---

## 4. UI State Completeness & Accessibility (R-27, R-32)

Every data-driven view must implement all three mandatory states:
1. **Loading State**: Subtle skeleton rows matching exact column dimensions. No pulsing rainbow gradients.
2. **Empty State**: Clear technical explanation when collection or query has zero records, accompanied by a primary creation CTA.
3. **Error State**: Explicit error message explaining the failure (e.g. network partition, 403 Forbidden, 409 Conflict) with a retry action.

### Keyboard & Focus Accessibility (R-32)
- Full keyboard navigation: `Tab` traverses interactive controls; `Escape` closes modals and slide-overs; Arrow keys navigate grid cells.
- Focus outlines must be visible (`focus-visible:ring-2 focus-visible:ring-primary`). Never use `outline: none` without an active ring replacement.
