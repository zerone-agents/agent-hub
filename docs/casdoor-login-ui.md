# Casdoor Login Page UI Configuration (Zerone Hub · Terracotta Theme)

Maps the project's design language (`frontend/src/styles/tokens.css` + `themes.ts` → Terracotta) to Casdoor application-level configuration.

---

## File Overview

| File | Purpose |
|------|------|
| **`casdoor-login.css`** | **The only style source file** — all CSS is maintained here |
| **`casdoor-footer.html`** | **The only footer HTML source file** — paste into the "Footer HTML" field |
| `casdoor-login-ui.md` | This document (configuration steps + design token mapping + troubleshooting) |

> **Maintenance rule**: To change styles, **only edit `casdoor-login.css`**; to change the footer, **only edit `casdoor-footer.html`**. Do not embed snippets in this markdown, and do not "append patches" in the Casdoor admin console.

---

## Configuration Steps

### 1. Field Settings (Application Edit Page → UI Customization Tab)

| Field | Value |
|------|-----|
| Primary color | `#D96F4F` |
| Border radius | `12` |
| Logo | `https://zerone-agent.oss-cn-hangzhou.aliyuncs.com/assets/zerone-logo-horizontal-v2.svg` |
| Form position | `Center` |
| Enable side panel | **Off** |
| Background URL | Leave empty (CSS already sets the background color) |
| Form CSS | **See step 2 below** |
| Form CSS (Mobile) | Leave empty (`@media` adaptation is already included) |
| Signup page HTML | **Leave empty** |
| Signin page HTML | **Leave empty** |
| Footer HTML | Paste the contents of `casdoor-footer.html` (replaces the default "Powered by Casdoor") |
| Per-row "Form CSS" in the signup/signin item tables | Leave as-is, do not modify |

### 2. Fill in the "Form CSS" Field

```bash
# One-click copy (the file already includes the <style> wrapper, ready to use)
cat docs/casdoor-login.css | pbcopy
```

Then go to the "Form CSS" field: **Select all → Clear → Paste → Save**.

### 3. Logo and Footer Images

- **Panel Logo**: Application edit page → Basics Tab → "Logo" field, fill in the OSS URL (see table above)
- **Footer Logo**: The `<img src>` in `casdoor-footer.html` already has the same OSS URL built in, no extra configuration needed

---

## Design Token Mapping

| Project token | Value | Usage in CSS |
|------|-----|------|
| `--card` (light) | `#fbfaf8` | `.login-panel` card background |
| `--background` (light) | `#f3f1ed` | Input background / autofill fill color |
| `--primary` (light) | `#d96f4f` | Buttons / links / focus border |
| `--primary-hover` | `#c86042` | Button hover |
| `--foreground` (light) | `#211f1d` | Input text |
| `--border` (light) | `#dcd7d0` | Card / input border |
| `--radius-lg` | `16px` | Card border radius |
| Card width | `fit-content` (min `440px`) | `.login-panel` adapts to content |
| Panel Logo | `280px` wide / `margin-bottom 18px` | `img.panel-logo` |
| Input | `40px` height | `.ant-input-affix-wrapper` etc. |
| Primary button | `40px` height / terracotta background | Login / signup submit buttons |
| Signup page label | Left-aligned / column width `100px` | `.ant-form-item-label` |
| Footer Logo | `32px` height | Inlined in `casdoor-footer.html` |
| `elevation-3` | `0 20px 50px rgba(33,31,29,.13)` | Card shadow |

---

## Maintenance Workflow

```
Spot a style issue
    ↓
Edit docs/casdoor-login.css (the only style source)
    ↓
cat docs/casdoor-login.css | pbcopy
    ↓
Casdoor admin → Form CSS → Select all → Clear → Paste → Save
    ↓
Refresh the login page to verify
```

Same flow for the footer:

```
Edit docs/casdoor-footer.html
    ↓
cat docs/casdoor-footer.html | pbcopy
    ↓
Casdoor admin → Footer HTML → Select all → Clear → Paste → Save
```

**Do not**:
- ❌ Embed CSS snippets in this markdown
- ❌ Append patches in the Casdoor admin console (causes inconsistency between source files and actual configuration)
- ❌ Modify the small per-row "Form CSS" in the signup/signin item tables (keep default)

---

## Verification Checklist

After replacing and saving, visit the login page `/login/oauth/authorize?...`:

### Login Page
- [ ] Single-layer card (auto-width, min 440px, warm-white background, 16px border radius)
- [ ] Panel logo 280px wide, 18px spacing below
- [ ] Inputs 40px tall, outer wrapper has border, **inner input fully transparent**
- [ ] After Chrome autofill, inputs stay warm-paper color (not blue)
- [ ] Login button 40px tall, terracotta background, hover darkens + lifts shadow
- [ ] "Forgot password" / "Sign up now" links in terracotta
- [ ] Footer shows Zerone horizontal logo (32px tall) + "Powered by"

### Signup Page `/signup`
- [ ] Card width adapts to content (label + input arranged horizontally without clipping)
- [ ] Field labels left-aligned, column width 100px
- [ ] "Send code" button has 8px spacing from the input

---

## Troubleshooting

| Symptom | Investigation |
|------|-----|
| CSS does not apply at all | F12 → Elements → `<head>` — check whether your `<style>` block is present |
| Some elements still default blue | F12 select that element → Styles panel → look at struck-through rules → add the selector to the corresponding place in `casdoor-login.css` |
| Footer logo appears huge | When inline style is filtered by Casdoor, the `height="32"` HTML attribute acts as fallback (`casdoor-footer.html` already includes it) |
| Logo image is wrong | CSS cannot change image content — edit the Logo field on the application edit page (panel) or `casdoor-footer.html` (footer) |

---

## Related Files

- Design tokens: [`frontend/src/styles/tokens.css`](../frontend/src/styles/tokens.css)
- Theme definitions: [`frontend/src/styles/themes.ts`](../frontend/src/styles/themes.ts)
- Reference implementation: [`frontend/src/features/login/LoginPage.tsx`](../frontend/src/features/login/LoginPage.tsx)
- Logo source file: [`frontend/public/favicon.svg`](../frontend/public/favicon.svg)
