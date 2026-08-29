import { marked, type Tokens } from 'marked'
import DOMPurify from 'dompurify'
import katex from 'katex'
import 'katex/dist/katex.min.css'
import hljs from 'highlight.js/lib/common'
// No stylesheet from highlight.js: it only ships fixed palettes, and importing
// one pinned every theme to it — on the four light themes that put dark-theme
// token colours on a light surface, where ten of fourteen measured under 3:1.
// style.css maps .hljs-* onto the --syn-* properties applySyntaxTheme() writes,
// so a fenced block is coloured by whatever theme is on.
import { t } from './i18n.svelte'
import { ICONS } from './icons'

marked.setOptions({ breaks: true, gfm: true })

// Which fence tags get a Run button beside Copy, as tag → kind, answered by the
// engine (App.RunnableLanguages, backed by internal/runlang) rather than held
// here.
//
// It is asked rather than known because the answer is about the machine, not
// about markdown: a `python` block is runnable on a computer with Python on it
// and is a picture of a program on one without. A list kept here could only ever
// say what is runnable in principle, and a Run button that comes back with a
// Microsoft Store advert is worse than no Run button.
//
// Empty until setRunnableLanguages is called, and empty is the safe direction:
// no button. A shell tag renders its text as the command; a script tag means a
// file, which the engine writes out and hands to an interpreter (run_script.go).
let runnable: Record<string, string> = {}

export function setRunnableLanguages(langs: Record<string, string>): void {
  runnable = langs
}

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
    const tag = language.toLowerCase()
    const kindOf = runnable[tag]
    const run = kindOf
      ? `<button class="code-run" type="button">${t('chat.runCode')}</button>`
      : ''
    // The tag rides on the block because the click handler has to know which
    // of the two kinds this is, and the label above is not it — an unknown
    // language renders as "code", and reading the tag back off a label that
    // has already been rewritten is how the two drift apart.
    const kind = kindOf === 'script' ? ` data-script="${escapeAttr(tag)}"` : ''
    return (
      `<div class="codeblock"${kind}>` +
      `<div class="codeblock-head"><span class="lang">${label}</span>` +
      run +
      `<button class="code-copy" type="button">${t('chat.copyCode')}</button></div>` +
      `<pre><code class="hljs">${highlighted}</code></pre>` +
      `</div>`
    )
  },
}
marked.use({ renderer })

// ---------- mathematics ----------
//
// A model asked for an integral writes LaTeX, because that is what mathematics
// is written in. Until now nothing here knew that, and markdown read it as
// prose — twice over. `\[` is a backslash-escaped bracket to markdown, so an
// equation opened with one arrived as a bare `[` on a line of its own; `x^2` and
// `a_1` handed their `^` and `_` to the emphasis parser; and whatever survived
// was printed as source. The screenshot that started this (owner, 16 ส.ค.) is
// exactly that: `\int_0^2 x^2\,dx` as text, between two orphaned brackets.
//
// So equations are tokenized before markdown reads anything — a marked
// extension runs ahead of the built-in tokenizers, which is what keeps the
// escape rule and the emphasis rule off the LaTeX inside — and rendered by
// KaTeX, which draws them.
//
// Four delimiters, because models write all four and which one arrives is not
// something the surface gets to choose: `\[...\]` and `$$...$$` for an equation
// on its own line, `\(...\)` and `$...$` for one inside a sentence.
//
// The display pair are inline tokenizers as well as block ones. A block
// tokenizer is only offered the start of a block, so `\[x\]` written mid
// paragraph — which happens — would otherwise fall through to the escape rule
// that this whole extension exists to get in front of.
//
// ARCHITECTURE.md §118.
const MATH = {
  blockDisplay: /^(?:\\\[([\s\S]+?)\\\]|\$\$([\s\S]+?)\$\$)(?:\n+|$)/,
  inlineDisplay: /^(?:\\\[([\s\S]+?)\\\]|\$\$([\s\S]+?)\$\$)/,
  // `\(...\)` is unambiguous. A single `$` is not — it is also how money is
  // written, and "$5 และ $10" is a sentence, not an equation named "5 และ ".
  // Three guards make the difference, and they are the ones every renderer
  // that has met this problem settles on: an equation does not open with a
  // space, does not close with one, and its closing `$` is not the start of
  // another price. A `$` cannot appear inside, so a run of prices can never be
  // joined into one match.
  inline: /^(?:\\\(([\s\S]+?)\\\)|\$(?!\s)((?:\\.|[^$\\])+?)(?<![\s\\])\$(?!\d))/,
} as const

interface MathToken extends Tokens.Generic {
  type: string
  raw: string
  tex: string
  display: boolean
}

function mathToken(type: string, display: boolean, match: RegExpExecArray): MathToken {
  return { type, raw: match[0], tex: (match[1] ?? match[2] ?? '').trim(), display }
}

const startDisplay = (src: string) => {
  const at = src.search(/\\\[|\$\$/)
  return at === -1 ? undefined : at
}
const startInline = (src: string) => {
  const at = src.search(/\\\(|\$/)
  return at === -1 ? undefined : at
}

marked.use({
  extensions: [
    {
      name: 'mathBlock',
      level: 'block',
      start: startDisplay,
      tokenizer(src: string) {
        const match = MATH.blockDisplay.exec(src)
        return match ? mathToken('mathBlock', true, match) : undefined
      },
      renderer: (token) => renderMath(token as MathToken),
    },
    // Ahead of mathInline in the list, so `$$x$$` inside a sentence is read as
    // one display equation rather than as two empty inline ones.
    {
      name: 'mathInlineDisplay',
      level: 'inline',
      start: startDisplay,
      tokenizer(src: string) {
        const match = MATH.inlineDisplay.exec(src)
        return match ? mathToken('mathInlineDisplay', true, match) : undefined
      },
      renderer: (token) => renderMath(token as MathToken),
    },
    {
      name: 'mathInline',
      level: 'inline',
      start: startInline,
      tokenizer(src: string) {
        const match = MATH.inline.exec(src)
        return match ? mathToken('mathInline', false, match) : undefined
      },
      renderer: (token) => renderMath(token as MathToken),
    },
  ],
})

// KaTeX refuses what it cannot read, and the answer to that is to hand back
// what the model wrote rather than a red error where the equation should be.
// A refusal here means one of two things and both end the same way: the text
// between the delimiters was never mathematics (a stray `$$` in prose), or it
// used a command KaTeX does not carry — and in either case the source is more
// use to the reader than an error message about it.
//
// Streaming is the other reason. Half an equation is a syntax error for the
// few frames before its closing brace arrives, and nothing should flash red on
// the way to being correct.
// KaTeX writes the equation twice: once as the spans that are drawn, and once
// as MathML for anything reading the page aloud. The MathML ends with an
// <annotation> holding the original LaTeX — a note for other tools, not for a
// reader — and DOMPurify deletes that tag while keeping its text, which is the
// worst of both: `\boxed{\text{...}}` left loose inside the <math> element, out
// of sight but read aloud and picked up by a copy.
//
// So it is dropped here instead, before the sanitizer can half-remove it. The
// MathML itself stays; it is the whole reason a screen reader can follow an
// equation whose visible half is marked aria-hidden.
const ANNOTATION = /<annotation\b[^>]*>[\s\S]*?<\/annotation>/g

// A display equation is drawn as a block of its own — a bordered surface with a
// header bar carrying a label and คัดลอก — which is the shape a fenced code
// block already has here (owner, 16 ส.ค.: "แสดงเหมือนตอนเจนโค้ดได้ไหม").
//
// It is the same kind of thing, which is why it gets the same box. Both are
// notation rather than prose: written in a language of their own, read as a
// unit rather than a line at a time, and wanted somewhere else as often as they
// are wanted here. The bare centred equation this replaces had the copy button
// floating over the notation with nothing to hang off.
//
// The label says `latex` for the same reason a code block's says `python`: it
// is the language of the thing, and it is exactly what คัดลอก will hand over.
// Untranslated, like the language tag, because it is a name and not a word.
//
// What it copies is the LaTeX the model wrote, carried on data-tex the way the
// plan card carries its markdown on data-plan. Reading it back off the rendered
// DOM instead would return KaTeX's own layout text — every fraction flattened,
// every limit and exponent run together on one line — which pastes into another
// tool as something that has to be retyped. The source pastes as the equation.
//
// Spans throughout and not divs: an equation written mid-paragraph renders
// inside a <p>, where a div would end the paragraph early and drop the rest of
// the sentence out of it. Inline maths gets none of this — a box around the `x`
// in the middle of a sentence is not a box, it is an interruption.
function frameEquation(html: string, tex: string): string {
  return (
    `<span class="math-block" data-tex="${escapeAttr(tex)}">` +
    `<span class="math-head"><span class="lang">latex</span>` +
    `<button class="math-copy" type="button">${escapeText(t('chat.copyCode'))}</button></span>` +
    `<span class="math-body">${html}</span>` +
    `</span>`
  )
}

function renderMath(token: MathToken): string {
  try {
    const html = katex.renderToString(token.tex, {
      displayMode: token.display,
      throwOnError: true,
      // `strict` is about LaTeX pedantry, not safety — on, it refuses things
      // like a Thai character inside `\text{}`, which is most of what the
      // equations in this app say. `trust` is the safety one and stays off: it
      // is what would let `\href` and `\includegraphics` out of the equation
      // and into the document.
      strict: false,
      trust: false,
    }).replace(ANNOTATION, '')
    return token.display ? frameEquation(html, token.tex) : html
  } catch {
    return escapeText(token.raw)
  }
}

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
  let nth = 0
  for (const svg of host.querySelectorAll('svg')) {
    if (svg.parentElement?.closest('svg')) continue
    // The app's own furniture — a card's icon — is not a drawing the user can
    // take away, and framing it would hang copy and save buttons off a 14px
    // glyph. Marked at the point it is built (renderPlan) rather than guessed
    // at here by size or by which element it sits in. Counted out of `nth` too,
    // so a plan card appearing above a drawing mid-stream cannot renumber it.
    if (svg.hasAttribute('data-chrome')) continue
    // KaTeX draws in SVG too — the bar over a square root, a stretched brace,
    // an arrow over a vector are all svg elements inside the equation. They are
    // parts of a letter, not pictures: framing one hangs คัดลอก and บันทึก
    // buttons off a radical sign, and renumbers the drawings around it.
    if (svg.closest('.katex')) continue
    confineDrawing(svg, nth++)
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
  for (const table of host.querySelectorAll('table')) {
    if (!table.closest('.table-scroll')) scrollTable(table)
  }
  return host.innerHTML
}

// A wide table has to scroll, and CSS alone could never make it.
//
// `.markdown-body table` has said `display:block; width:max-content;
// max-width:100%; overflow-x:auto` since the day a 7-column comparison rendered
// "anthropic" as "anthro/pic" and "200000" as "2000/00". The comment there
// claims those declarations let a wide table scroll instead of squeezing.
// Measured on 2026-08-27, in a browser, against the reading column's real
// width: they do not. `max-width:100%` clamps the used width to the column and
// the table then lays its columns out INSIDE that clamp, so scrollWidth comes
// back equal to clientWidth and there is nothing to scroll. A 1,994px table
// reported 860 / 860 and simply lost its last column off the right edge, which
// is what the owner screenshotted.
//
// The element that scrolls cannot be the element that sizes itself to its
// content, so it has to be a second element, and CSS cannot add one. Wrapped
// here, where the sanitised DOM is already being walked for drawings and
// stylesheets. The table goes back to being a real `display:table` at its
// natural width; the wrapper is the window onto it.
//
// Guarded against re-wrapping so a re-render of the same content cannot nest
// windows inside windows, and skipped for a table already inside one.
function scrollTable(table: Element): void {
  const win = document.createElement('div')
  win.className = 'table-scroll'
  table.replaceWith(win)
  win.appendChild(table)
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
  capEnlargement(svg)
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

// capEnlargement is the other half of the framed stage (.drawing-box in
// style.css). The stage is one width for every drawing so that a transcript
// stops looking like the app cannot draw the same picture twice, and the svg
// fills it — but "fills it" on its own is the 2026-08-14 bug wearing a frame:
// a drawing composed at 380 units stretched across a 644px stage is 1.7x, and
// every label in it is 1.7x too.
//
// CSS cannot see a viewBox, so the ceiling is written here, where it can. The
// drawing may grow by up to a sixth of what it was composed at and no further:
// enough that a drawing laid out at anything near the stage width simply fills
// it, and a small one is enlarged slightly rather than blown up. Under that it
// stays its own size, centred on the stage.
//
// No viewBox and no width attribute means nothing to reason from — width:auto
// in the stylesheet keeps that case at its intrinsic size, which is the
// behaviour it already had.
const maxDrawingGrowth = 1.16

function capEnlargement(svg: Element): void {
  const composed = composedWidth(svg)
  if (composed === 0) return
  const cap = `max-width:${Math.round(composed * maxDrawingGrowth)}px`
  const own = svg.getAttribute('style') ?? ''
  svg.setAttribute('style', own === '' ? cap : `${own};${cap}`)
}

// The width the model laid the drawing out at: the third number of the viewBox,
// or a plain pixel width attribute when there is no viewBox. A percentage width
// is what the prompt asks for and says nothing about size, so it is not one.
function composedWidth(svg: Element): number {
  const box = (svg.getAttribute('viewBox') ?? '').trim().split(/[\s,]+/)
  if (box.length === 4) {
    const width = Number(box[2])
    if (Number.isFinite(width) && width > 0) return width
  }
  const attr = (svg.getAttribute('width') ?? '').trim()
  if (attr !== '' && !attr.endsWith('%')) {
    const width = Number.parseFloat(attr)
    if (Number.isFinite(width) && width > 0) return width
  }
  return 0
}

function confineDrawing(svg: Element, key: string | number): void {
  const styles = Array.from(svg.querySelectorAll('style'))
  const identified = Array.from(svg.querySelectorAll('[id]'))
  if (styles.length === 0 && identified.length === 0) return

  // Taken from the opening tag and the drawing's position, NOT from the whole
  // of it. The whole of it grows with every token while the drawing streams, so
  // a fingerprint of it renamed every id and every animation on every frame —
  // and a <style> rewritten each frame restarts the animations it names, which
  // is a model's animated drawing stuck on its first frame forever. The opening
  // tag is complete before the first shape is drawn (renderStreamingMarkdown
  // waits for it), so this is settled from the first frame and stays settled.
  //
  // Position is what keeps two drawings apart when they open identically, which
  // is the collision the fingerprint exists for in the first place.
  const openTag = svg.outerHTML.slice(0, svg.outerHTML.indexOf('>') + 1)
  const scope = fingerprint(`${key}|${openTag}`)
  svg.setAttribute('data-drawing', scope)

  const renamed = new Map<string, string>()
  for (const el of identified) {
    const old = el.getAttribute('id') ?? ''
    if (old === '') continue
    const next = `${scope}-${old}`
    renamed.set(old, next)
    el.setAttribute('id', next)
  }
  const anims = new Map<string, string>()
  for (const style of styles) {
    for (const name of keyframeNames(style.textContent ?? '')) {
      if (!anims.has(name)) anims.set(name, `${scope}-${name}`)
    }
  }
  if (renamed.size > 0 || anims.size > 0) {
    for (const el of [svg, ...svg.querySelectorAll('*')]) {
      for (const attr of Array.from(el.attributes)) {
        if (attr.name === 'id') continue
        // An animation is as likely to be started from a style attribute as
        // from the stylesheet — the prompt already tells the model to colour
        // that way — and a name renamed in one place and not the other is an
        // animation that silently does nothing.
        const next = attr.name === 'style'
          ? renameAnimations(rename(attr.value, renamed), anims)
          : rename(attr.value, renamed)
        if (next !== attr.value) el.setAttribute(attr.name, next)
      }
    }
  }
  for (const style of styles) {
    style.textContent = scopeCss(rename(style.textContent ?? '', renamed), `[data-drawing="${scope}"] `, anims)
  }
}

/** A whole .svg file, made safe to put in the app's own document.
 *
 * The file pane used to point an <img> at the file and call that showing it.
 * Nothing about that works for the drawings this app asks for: internal/prompt
 * teaches `width="100%"` and var(--surface-raised), and an <img> is a separate
 * document, so the width resolved against nothing (0×0, an empty pane) and
 * every var() resolved to black on black. A model doing exactly what it was
 * told, written to a file, and shown as nothing (owner, 29 ส.ค.).
 *
 * Inlining is what gives it back both — the app's stylesheet and a real box —
 * and it comes through the same door a drawing in an answer does, because the
 * hazards are the same file: DOMPurify for scripts and handlers, confineDrawing
 * for a <style> that would style the app and for ids that would collide with
 * the next .svg opened beside it. `key` is the file's path rather than a
 * position, so two files never share a scope and one file keeps its own across
 * a re-read.
 *
 * Returns '' for anything with no <svg> in it, which is the pane's cue to say
 * the file is not a drawing rather than to draw an empty box. */
export function inlineDrawing(markup: string, key: string): string {
  const host = document.createElement('div')
  host.appendChild(DOMPurify.sanitize(markup, { RETURN_DOM_FRAGMENT: true }))
  // Same rule as confine(): a stylesheet with a drawing to scope it to is
  // scoped, one with nothing to scope it to is deleted.
  for (const style of host.querySelectorAll('style')) {
    if (!style.closest('svg')) style.remove()
  }
  const svg = host.querySelector('svg')
  if (!svg) return ''
  confineDrawing(svg, key)
  return svg.outerHTML
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

// An animation name is document-wide in exactly the way an id is, and a model
// names its keyframes `spin`. Two drawings in one answer both calling theirs
// that, and the second one plays the first one's animation — the same collision
// the id fingerprint already exists to prevent, so it gets the same fix.
const KEYFRAMES = /@(?:-webkit-)?keyframes\s+(-?[A-Za-z_][\w-]*)/gi

function keyframeNames(css: string): string[] {
  return Array.from(css.matchAll(KEYFRAMES), (m) => m[1])
}

// Only whole identifiers move, and only in the two places a keyframes name is
// ever written: the `@keyframes` head, and an `animation` / `animation-name`
// value. Both are handled by matching the word itself — a name we saw declared
// in this same drawing — rather than by parsing the shorthand, whose order is
// free and whose other tokens (a duration, an easing, `infinite`) are not
// identifiers a model would also have named its keyframes.
function renameAnimations(text: string, anims: Map<string, string>): string {
  let out = text
  for (const [old, next] of anims) {
    out = out.replace(
      new RegExp(`(^|[^\\w-])${old.replace(/[.*+?^${}()|[\]\\-]/g, '\\$&')}(?![\\w-])`, 'g'),
      (_m, lead: string) => lead + next,
    )
  }
  return out
}

// Prefixes every selector in a stylesheet so it can only match inside one
// drawing. Nesting at-rules (@media and friends) keep their block and have the
// rules inside it prefixed; the rest — @import above all, which fetches from
// the network on the strength of model output — are dropped whole.
//
// @keyframes is the exception that is neither. Its block is not a set of
// selectors — the heads inside are `0%`, `from`, `to` — so prefixing them would
// build rules matching nothing and a drawing whose animation never moves, which
// is what a model's animated SVG did here until 15 ส.ค. It is emitted whole and
// only its name is scoped.
//
// @property stays dropped, deliberately: a registered custom property is
// global by definition, with no boundary to confine it to, so a drawing cannot
// have one without reaching into the app's document.
function scopeCss(css: string, prefix: string, anims: Map<string, string>): string {
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
        out += `${head}{${scopeCss(body, prefix, anims)}}`
      } else if (KEYFRAMES.test(head)) {
        KEYFRAMES.lastIndex = 0 // the regex is global; a stale index skips the next head
        out += `${renameAnimations(head, anims)}{${body}}`
      }
    } else if (head !== '') {
      const selectors = head
        .split(',')
        .map((s) => s.trim())
        .filter((s) => s !== '')
        .map((s) => prefix + s)
        .join(',')
      out += `${selectors}{${renameAnimations(body, anims)}}`
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
  if (open === -1 || text.slice(open).includes('</svg>')) return markLive(renderMarkdown(text), text, false)

  const openTagEnd = text.indexOf('>', open)
  // The drawing has not started drawing yet, so there is nothing to mark alive.
  if (openTagEnd === -1) return markLive(renderMarkdown(text.slice(0, open)), text, false)

  const lastElement = text.lastIndexOf('<')
  const whole = text.indexOf('>', lastElement) === -1 ? text.slice(0, lastElement) : text
  return markLive(renderMarkdown(whole + '</svg>'), text, true)
}

// Marks the one block still being written, so the surface can show it is being
// written (style.css, the running beam).
//
// A block that takes a while to arrive — a plan, a drawing, a long fenced file
// — reads as finished from its first frame: the card has its edge, its heading
// and its คัดลอก button before a third of it exists, and a user who copies then
// gets a third of it. The delegation card had this exact problem and was given
// a beam; this is the same signal on the same grounds (owner, 15 ส.ค.).
//
// Which block is unfinished is not guessed. A drawing's opening <svg> with no
// </svg> yet is what the caller above already computed; a fence is open when
// the count of fence lines is odd. Only the LAST match is marked, because the
// earlier ones closed — three code blocks in an answer must not all pulse
// because the fourth is arriving.
//
// The finished message renders through renderMarkdown, which does none of
// this, so a block cannot be left glowing by a turn that ended.
function markLive(html: string, source: string, drawingOpen: boolean): string {
  const selector = drawingOpen ? '.drawing-box' : fenceOpen(source) ? '.plan-card,.codeblock' : ''
  if (selector === '') return html
  const host = document.createElement('div')
  host.innerHTML = html
  const blocks = host.querySelectorAll(selector)
  const last = blocks[blocks.length - 1]
  if (!last) return html
  last.classList.add('live')
  return host.innerHTML
}

// Up to three leading spaces is still a fence to markdown; four makes it an
// indented code block, which has no opening line to count.
function fenceOpen(source: string): boolean {
  return (source.match(/^ {0,3}(?:```|~~~)/gm) ?? []).length % 2 === 1
}
