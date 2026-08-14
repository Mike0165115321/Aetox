import { marked, type Tokens } from 'marked'
import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/common'
// No stylesheet from highlight.js: it only ships fixed palettes, and importing
// one pinned every theme to it — on the four light themes that put dark-theme
// token colours on a light surface, where ten of fourteen measured under 3:1.
// style.css maps .hljs-* onto the --syn-* properties applySyntaxTheme() writes,
// so a fenced block is coloured by whatever theme is on.
import { t } from './i18n.svelte'
import { ICONS } from './icons'

marked.setOptions({ breaks: true, gfm: true })

// Languages whose fenced blocks get a Run button beside Copy. Only tags that
// unambiguously mean "a command for this machine's shell" — `console` and
// `text` are for showing output, `python` and friends are source files, and
// running either would be a wrong guess with a real side effect.
const runnableLangs = new Set(['bash', 'sh', 'shell', 'zsh', 'powershell', 'ps1', 'cmd', 'bat'])

// Fenced code blocks render like a normal AI chat: a header bar with the
// language label and a copy button, plus syntax highlighting. Shell-tagged
// blocks also get a Run button — the same affordance Claude Code puts on
// them. Both buttons' clicks are handled by delegation in Chat.svelte
// (markup from {@html} can't carry Svelte handlers).
const renderer = {
  code({ text, lang }: Tokens.Code): string {
    const language = (lang ?? '').trim().split(/\s+/)[0]
    // A plan is not source, so it does not get a code block. internal/prompt's
    // `planCard` layer is what asks the model for this fence; see renderPlan.
    if (language.toLowerCase() === 'plan' && !insidePlan) return renderPlan(text)
    const known = language !== '' && hljs.getLanguage(language) !== undefined
    const highlighted = known
      ? hljs.highlight(text, { language }).value
      : hljs.highlightAuto(text).value
    const label = known ? language : 'code'
    const run = runnableLangs.has(language.toLowerCase())
      ? `<button class="code-run" type="button">${t('chat.runCode')}</button>`
      : ''
    return (
      `<div class="codeblock">` +
      `<div class="codeblock-head"><span class="lang">${label}</span>` +
      run +
      `<button class="code-copy" type="button">${t('chat.copyCode')}</button></div>` +
      `<pre><code class="hljs">${highlighted}</code></pre>` +
      `</div>`
    )
  },
}
marked.use({ renderer })

// A plan arrives as a fenced block tagged `plan` and is drawn as a card of its
// own: the icon of the dial that produced it, a title, and the plan itself
// (internal/prompt.planCard asks for exactly this, and only where a surface can
// draw it — a terminal session gets the same plan with no fence around it).
//
// The fence is the contract because this renderer already had the seam: a code
// block gets a header bar and a copy button by being intercepted here on its
// language. A plan is that move with a different box, which is why the card
// costs a branch rather than a parser.
//
// Rendered as a *card and not a bubble of its own*: it stays inside the
// assistant's message, so a plan that follows a sentence still reads in order.
// It is deliberately not collapsed behind a toggle either. In วางแผน the plan
// is not an artifact beside the answer — it *is* the answer, and the turn was
// spent producing it, so hiding it behind a click would hide the whole reply.
//
// insidePlan stops a `plan` fence written *inside* a plan from recursing: the
// inner one renders as an ordinary code block, which is also what it looks
// like when a model is showing the user how the fence is written.
let insidePlan = false

function renderPlan(source: string): string {
  const { title, body } = splitPlanTitle(source)
  insidePlan = true
  let inner: string
  try {
    inner = marked.parse(body, { async: false }) as string
  } finally {
    insidePlan = false
  }
  // data-chrome marks this svg as the app's own furniture rather than something
  // the model drew, so confine() leaves it alone — without it the card's icon
  // is framed as a drawing, complete with its own copy and save buttons.
  const icon =
    `<svg class="icon" data-chrome="1" width="14" height="14" viewBox="0 0 24 24" fill="none" ` +
    `stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" ` +
    `aria-hidden="true">${ICONS.compass}</svg>`
  // The source rides along in an attribute so Copy hands back the markdown the
  // model wrote — the thing a user pastes into an issue or a commit message.
  // Reading it back off the rendered DOM instead would return the card's text
  // with every heading and bullet flattened out of it.
  const heading = title === '' ? '' : `<h3 class="plan-title">${marked.parseInline(title, { async: false }) as string}</h3>`
  return (
    `<div class="plan-card" data-plan="${escapeAttr(source)}">` +
    `<div class="plan-head">` +
    `<span class="plan-kind">${icon}${escapeText(t('chat.planCard'))}</span>` +
    `<button class="plan-copy" type="button">${escapeText(t('chat.copyCode'))}</button>` +
    `</div>` +
    heading +
    `<div class="plan-body">${inner}</div>` +
    `</div>`
  )
}

// The plan's title is the first heading inside the fence, and it is lifted out
// rather than left in the body: it is the card's own line, drawn above the
// plan the way the desk name is drawn above a session. Anything else — a plan
// whose first line is already prose — keeps its text and simply has no title,
// which reads as a card rather than as a mistake.
function splitPlanTitle(source: string): { title: string; body: string } {
  const lines = source.split('\n')
  let i = 0
  while (i < lines.length && lines[i].trim() === '') i++
  const head = lines[i]?.match(/^\s{0,3}#{1,3}\s+(.+?)\s*#*\s*$/)
  if (!head) return { title: '', body: source }
  return { title: head[1], body: lines.slice(i + 1).join('\n') }
}

function escapeText(value: string): string {
  return value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function escapeAttr(value: string): string {
  return escapeText(value).replace(/"/g, '&quot;')
}

// A drawing is markup inside prose, and markdown is entitled to read markup as
// prose. Two of its rules bite a hand-written <svg>:
//
//   - A blank line ends an HTML block. The next line, indented four spaces or a
//     tab the way anyone pretty-prints XML, is then a code block — the drawing
//     is cut off at the blank line and its remaining shapes are printed as
//     source next to it.
//   - An <svg> that opens on the line under a sentence is inline HTML, so its
//     own text runs through the inline parser: `*` around a label becomes <em>
//     and is lifted out of the drawing.
//
// So a complete drawing is lifted out before markdown ever sees it and put back
// after — the same treatment a fenced block gets, for the same reason.
//
// Only a drawing in the left margin is lifted. One written inside backticks is
// being discussed rather than drawn, and it starts after the backtick; an
// indented one belongs to the list item it is indented under, and lifting it
// would land the picture below the list instead of inside the bullet.
const DRAWING = /^<svg\b[\s\S]*?<\/svg\s*>/gim
const PLACEHOLDER = /<!--aetox-drawing-(\d+)-->/g

// Fenced blocks are the one place an <svg> at the start of a line is source
// code the user asked to see, not a picture.
function fencedSpans(text: string): Array<[number, number]> {
  const spans: Array<[number, number]> = []
  let open = -1
  let at = 0
  for (const line of text.split('\n')) {
    if (/^ {0,3}(`{3,}|~{3,})/.test(line)) {
      if (open === -1) open = at
      else { spans.push([open, at + line.length]); open = -1 }
    }
    at += line.length + 1
  }
  if (open !== -1) spans.push([open, text.length])
  return spans
}

function liftDrawings(text: string): { text: string; held: string[] } {
  const fences = fencedSpans(text)
  const held: string[] = []
  const lifted = text.replace(DRAWING, (match, offset: number) => {
    if (fences.some(([from, to]) => offset >= from && offset < to)) return match
    return `<!--aetox-drawing-${held.push(match) - 1}-->`
  })
  return { text: lifted, held }
}

// Chat text comes from the model (and the user's own draft) — never trust it
// as HTML directly, sanitize after markdown expansion.
//
// DOMPurify passes SVG and strips script elements and on* handlers, which is
// what makes a drawing in an answer safe to render at all (internal/prompt's
// `drawing` layer is what asks for them). Nothing here has to allow it — the
// point of the note is that nothing may quietly forbid it either.
export function renderMarkdown(text: string): string {
  const { text: lifted, held } = liftDrawings(text)
  const html = (marked.parse(lifted, { async: false }) as string).replace(
    PLACEHOLDER,
    (_, i: string) => held[Number(i)] ?? ''
  )
  return confine(html)
}

// What DOMPurify guards is the machine: no script, no handler, no way for a
// displayed answer to act. What it does not guard is the app around the answer,
// and two things a drawing carries reach outside its own box.
//
// A <style> inside an inline <svg> is not scoped to that svg — it is a
// stylesheet in the document, and `.row{display:none}` written by a model
// aiming at its own legend hides the sidebar. Its rules are prefixed here so
// they can only match inside the drawing that brought them.
//
// An id is document-wide too, and `url(#g)` resolves to the first match in the
// page: two drawings that both name a gradient `g` — which is what a model
// names one — leave the second drawing wearing the first one's colours. Every
// id in a drawing is prefixed with a fingerprint of the drawing itself, so the
// only ids that can still collide belong to drawings that are identical.
//
// Both passes are skipped by a drawing that has neither, which is most of them.
function confine(html: string): string {
  const host = document.createElement('div')
  host.appendChild(DOMPurify.sanitize(html, { RETURN_DOM_FRAGMENT: true }))
  for (const svg of host.querySelectorAll('svg')) {
    if (svg.parentElement?.closest('svg')) continue
    // The app's own furniture — a card's icon — is not a drawing the user can
    // take away, and framing it would hang copy and save buttons off a 14px
    // glyph. Marked at the point it is built (renderPlan) rather than guessed
    // at here by size or by which element it sits in.
    if (svg.hasAttribute('data-chrome')) continue
    confineDrawing(svg)
    frameDrawing(svg)
  }
  // A <style> outside a drawing is deleted rather than scoped.
  //
  // At the top level DOMPurify already drops it; nested in a <div> it does not,
  // and a panel (internal/prompt's `panel` layer) is divs — so a model reaching
  // for a stylesheet the way it would on a web page was writing one into the
  // app's document, where `.row{display:none}` hides the sidebar. A drawing's
  // <style> is scoped instead of deleted because a drawing has a boundary to
  // scope it to; a panel is arbitrary markup with none, and the prompt already
  // tells the model to colour through style attributes, which is the thing that
  // survives and also the thing that reads the user's theme.
  for (const style of host.querySelectorAll('style')) {
    if (!style.closest('svg')) style.remove()
  }
  return host.innerHTML
}

// frameDrawing puts the take-it-with-you controls on a drawing: copy and save,
// revealed on hover, the same affordance a code block's header gives its text.
// A drawing that cannot leave the bubble is a chart the user re-makes by hand
// in another tool. Clicks are handled by delegation in Chat.svelte, exactly as
// the code buttons' are — {@html} markup can't carry Svelte handlers.
function frameDrawing(svg: Element): void {
  const box = document.createElement('div')
  box.className = 'drawing-box'
  svg.replaceWith(box)
  box.appendChild(svg)
  const tools = document.createElement('div')
  tools.className = 'drawing-tools'
  for (const [cls, label] of [
    ['drawing-copy', t('chat.copyDrawing')],
    ['drawing-save', t('chat.saveDrawing')],
  ]) {
    const button = document.createElement('button')
    button.type = 'button'
    button.className = cls
    button.textContent = label
    tools.appendChild(button)
  }
  box.appendChild(tools)
}

function confineDrawing(svg: Element): void {
  const styles = Array.from(svg.querySelectorAll('style'))
  const identified = Array.from(svg.querySelectorAll('[id]'))
  if (styles.length === 0 && identified.length === 0) return

  const scope = fingerprint(svg.outerHTML)
  svg.setAttribute('data-drawing', scope)

  const renamed = new Map<string, string>()
  for (const el of identified) {
    const old = el.getAttribute('id') ?? ''
    if (old === '') continue
    const next = `${scope}-${old}`
    renamed.set(old, next)
    el.setAttribute('id', next)
  }
  if (renamed.size > 0) {
    for (const el of [svg, ...svg.querySelectorAll('*')]) {
      for (const attr of Array.from(el.attributes)) {
        if (attr.name === 'id') continue
        const next = rename(attr.value, renamed)
        if (next !== attr.value) el.setAttribute(attr.name, next)
      }
    }
  }
  for (const style of styles) {
    style.textContent = scopeCss(rename(style.textContent ?? '', renamed), `[data-drawing="${scope}"] `)
  }
}

// `url(#g)` in a fill or a stylesheet, and `#g` as a whole href — the two ways
// one part of a drawing points at another. A bare `#g` elsewhere in an
// attribute is a colour or a fragment link, so only these two shapes move.
function rename(value: string, renamed: Map<string, string>): string {
  return value
    .replace(/url\(\s*(['"]?)#([^)'"\s]+)\1\s*\)/g, (match, quote: string, id: string) => {
      const next = renamed.get(id)
      return next === undefined ? match : `url(${quote}#${next}${quote})`
    })
    .replace(/^#([^\s]+)$/, (match, id: string) => {
      const next = renamed.get(id)
      return next === undefined ? match : `#${next}`
    })
}

// Prefixes every selector in a stylesheet so it can only match inside one
// drawing. Nesting at-rules (@media and friends) keep their block and have the
// rules inside it prefixed; the rest — @import above all, which fetches from
// the network on the strength of model output — are dropped whole.
function scopeCss(css: string, prefix: string): string {
  let out = ''
  let at = 0
  while (at < css.length) {
    const open = css.indexOf('{', at)
    if (open === -1) break
    const head = css.slice(at, open).trim()
    const close = blockEnd(css, open)
    const body = css.slice(open + 1, close)

    if (head.startsWith('@')) {
      if (/^@(media|supports|layer|scope|container)\b/i.test(head)) {
        out += `${head}{${scopeCss(body, prefix)}}`
      }
    } else if (head !== '') {
      const selectors = head
        .split(',')
        .map((s) => s.trim())
        .filter((s) => s !== '')
        .map((s) => prefix + s)
        .join(',')
      out += `${selectors}{${body}}`
    }
    at = close + 1
  }
  return out
}

function blockEnd(css: string, open: number): number {
  let depth = 0
  for (let i = open; i < css.length; i++) {
    if (css[i] === '{') depth++
    else if (css[i] === '}' && --depth === 0) return i
  }
  return css.length
}

// FNV-1a. Not a security property — it only has to differ between two drawings
// that name the same id, and match between two renders of the same drawing so a
// streaming frame does not renumber what the previous one drew.
function fingerprint(text: string): string {
  let h = 0x811c9dc5
  for (let i = 0; i < text.length; i++) {
    h ^= text.charCodeAt(i)
    h = Math.imul(h, 0x01000193)
  }
  return 'd' + (h >>> 0).toString(36)
}

// A drawing arrives one token at a time, and it is drawn as it arrives — the
// shapes appear one by one and the picture builds itself.
//
// What makes that watchable rather than seasick is the viewBox, which is in
// the opening tag and therefore arrives before any shape does. With it and
// width="100%" the box has its final size from the first frame, so the shapes
// fill a space that is already the right shape instead of shoving the reply
// down the page as each one lands. That is the whole trick, and it is why
// internal/prompt's `drawing` layer insists on both attributes.
//
// Two things are trimmed off the live text before it is rendered:
//
//   - The half-written element at the very end. A `<rect width="60` fed to the
//     parser gets an attribute built out of whatever follows it, including the
//     closing tag added below.
//   - Everything, while the opening tag itself is still arriving. Until it
//     closes there is no viewBox, and an unsized drawing is the jumping this
//     is all here to avoid. It lasts a few tokens.
//
// Only the last opening tag is looked for, not a matched pair — nested <svg>
// is not something a drawing in an answer does, and a scan that balanced tags
// would run on every chunk of every reply for a case that never arrives.
export function renderStreamingMarkdown(text: string): string {
  const open = text.lastIndexOf('<svg')
  if (open === -1 || text.slice(open).includes('</svg>')) return renderMarkdown(text)

  const openTagEnd = text.indexOf('>', open)
  if (openTagEnd === -1) return renderMarkdown(text.slice(0, open))

  const lastElement = text.lastIndexOf('<')
  const whole = text.indexOf('>', lastElement) === -1 ? text.slice(0, lastElement) : text
  return renderMarkdown(whole + '</svg>')
}
