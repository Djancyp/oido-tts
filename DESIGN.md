# Design System: Oido TTS

## 1. Visual Theme & Atmosphere
A private, on-device audio studio — not a marketing site, not a chat app.
The feel is a well-lit mastering booth: calm, precise, a little clinical,
built for someone who will stare at this screen for long synthesis
sessions. Density is "Daily App Balanced" leaning dense (6/10) — this is a
working tool with a sidebar, a script editor, and waveform feedback, not
an airy landing page. Variance is moderate (5/10): the two-view structure
(Compose / Podcast) stays structurally consistent, but asymmetric
sidebar-vs-canvas layout avoids the centered-card-in-void look. Motion is
restrained-fluid (5/10) — waveforms breathe, buttons give tactile
feedback, but nothing distracts from a user reading or editing text while
audio renders in the background.

## 2. Color Palette & Roles
- **Studio Canvas** (#FAFAF9) — Primary app background, warm-neutral off-white (light mode)
- **Charcoal Depth** (#121212) — Primary app background (dark mode), never pure black
- **Panel Surface** (#FFFFFF / #1C1C1C) — Sidebar, cards, script editor surface
- **Ink** (#1C1B1A) — Primary text, light mode
- **Bone** (#EDEBE8) — Primary text, dark mode
- **Graphite** (#6B6864) — Secondary text, metadata, timestamps, helper copy
- **Seam** (rgba(28,27,26,0.08)) — Borders, dividers, waveform track background
- **Signal Amber** (#C9762B) — Single accent: record button, active waveform fill, primary CTA, focus rings. Desaturated amber, not neon orange — reads as "recording/live signal" without screaming
(Max 1 accent. Saturation < 80%. No purple/blue neon anywhere near the record or synthesize actions — those are exactly the buttons that must never look "AI-generated.")

## 3. Typography Rules
- **Display/Headers:** `Geist` — track-tight, weight-driven hierarchy (Compose/Podcast tab labels, section headers). No headline exceeds 1.5rem; this is a tool, not a hero.
- **Body/UI:** `Geist` — relaxed leading for the script/text-input area, 65ch soft max-width on any prose (e.g. help text), left-aligned always.
- **Mono:** `Geist Mono` — mandatory for anything numeric or state-like: duration timestamps, chunk counters ("3/12"), file paths, model names, cache hit/miss badges, waveform time markers.
- **Banned:** Inter (default system look), any serif anywhere (this is a dashboard-class UI, serif is banned outright, no editorial exception).

## 4. Component Stylings
* **Buttons:** Flat fills, 8px corner radius. Primary (Synthesize/Record) uses Signal Amber fill with a -1px tactile translate + shadow compression on active — no glow, ever. Secondary actions are ghost/outline in Graphite. Destructive (delete voice clip) uses a muted red, never amber.
* **Record button specifically:** a filled circle, not a rectangle — the one deliberately "different" shape in the whole UI, since it's the single most-clicked control. Pulses (opacity 0.6→1, 1.8s loop) only while actively recording.
* **Cards:** Used sparingly — only for the voice-clone picker and saved-voice list, where elevation communicates "this is a distinct, selectable item." 12px radius, whisper shadow tinted toward Ink at 4% opacity, 1px Seam border. The Compose/Podcast main canvas is NOT a card — it's the page itself, bordered only by the sidebar rail.
* **Waveform display:** the signature component. Bars/path in Signal Amber over Seam-colored track, animates in on synthesis complete via staggered bar-height reveal (60ms cascade), not an instant paint-in.
* **Inputs:** Label above input, helper text below in Graphite, error text in muted red below that. The main text-to-speech textarea has no visible border until focused (just a Seam bottom rule) — it should feel like writing, not filling a form.
* **Loaders:** While a chunk synthesizes, show a skeletal waveform (flat animated shimmer bar at the target height) — never a spinner. Chunk progress shown as a mono counter, not a percentage bar.
* **Empty states:** Podcast view with no script yet shows a composed example ("Alice: Welcome back.\nBob: Great to be here.") in placeholder-gray mono text, not a blank textarea with a gray hint line.
* **Sidebar (icon rail):** Collapsed by default per the existing left icon rail — icons in Graphite, active icon in Ink with a 2px Signal Amber left-edge indicator, never a filled-background active state.

## 5. Layout Principles
- Two-pane structure: fixed-width icon rail (already implemented) + fluid main canvas. No third column, no floating panels.
- Compose view: asymmetric split — text editor takes ~60% width, voice/settings controls occupy a right-hand panel (~40%), never stacked centrally.
- Podcast view: script editor full-width above, speaker-voice assignment as a horizontal row of compact cards below — not a vertical list, not a 3-equal-column grid.
- CSS Grid for the two-pane and split layouts; no flexbox percentage math, no `calc()` hacks.
- Contain the main canvas at a max-width of 960px when window is very wide (desktop app can be maximized on ultrawide monitors) — center it in the available space rather than stretching text lines past readable width.
- Full-height panes use `min-h-[100dvh]` equivalent (or the desktop-window equivalent: flex-1 with explicit min-height), never a hardcoded viewport unit that breaks on window resize.

## 6. Responsive Rules
This ships as a native desktop window (Wails), not a public webpage, but the window is user-resizable and must not break at narrow widths:
- Below ~720px window width: right-hand settings panel in Compose collapses into a slide-over drawer triggered by a gear icon, rather than squeezing into a sliver column.
- No horizontal scroll ever, at any window size — clip and wrap text areas instead.
- Minimum tap/click target 32px (desktop pointer context, not mobile touch) but keep 44px for the record button and primary Synthesize button specifically, since misclicks there are costly (re-recording, re-running synthesis).
- Sidebar icon rail stays fixed-width and never collapses further — it's already minimal.

## 7. Motion & Interaction
- Spring physics (stiffness ~120, damping ~22) for panel open/close (settings drawer, voice picker) — weighty, not bouncy.
- The waveform is the one "perpetual" element: a subtle idle-state breathing animation (opacity 0.85→1, 3s loop) on the record button only while armed but not yet recording, signaling "ready."
- Chunk-by-chunk synthesis progress reveals each completed chunk's waveform segment with a staggered cascade (matches the sequential, one-chunk-at-a-time backend reality — the animation should be honest about the actual pipeline, not fake a parallel fill).
- Animate only `transform` and `opacity`. Never animate `width`/`height` on the waveform bars themselves — precompute bar heights and transform-scale them from a 0 baseline instead.

## 8. Anti-Patterns (Banned)
- No emojis anywhere in the UI
- No `Inter` font
- No serif fonts (dashboard-class UI, no exceptions)
- No pure black (#000000) — use Charcoal Depth (#121212)
- No neon/outer glow on the record or synthesize buttons — this is the one place "looks AI-generated" would actively undermine user trust in a privacy-focused, on-device app
- No purple/blue gradient anywhere
- No circular spinners — skeletal waveform shimmer instead
- No generic 3-equal-card feature rows
- No fake precision in stats (no "99.9% local" claims — this app either sends data off-device or it doesn't; state facts, not marketing rounding)
- No AI copywriting clichés ("Elevate your voice," "Seamless synthesis," "Unleash") — copy should read like tool documentation, not a landing page
- No filler UI text ("scroll to explore," bouncing chevrons) — irrelevant to a desktop app anyway, but flagging so it never creeps in
- No centered hero/empty-state compositions — align left/asymmetric per section 4-5
