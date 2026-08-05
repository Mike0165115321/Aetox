import { marked, type Tokens } from 'marked'
import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/common'
// No stylesheet from highlight.js: it only ships fixed palettes, and importing
// one pinned every theme to it — on the four light themes that put dark-theme
// token colours on a light surface, where ten of fourteen measured under 3:1.
// style.css maps .hljs-* onto the --syn-* properties applySyntaxTheme() writes,
// so a fenced block is coloured by whatever theme is on.
import { t } from './i18n.svelte'

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

// Chat text comes from the model (and the user's own draft) — never trust it
// as HTML directly, sanitize after markdown expansion.
//
// DOMPurify passes SVG and strips script elements and on* handlers, which is
// what makes a drawing in an answer safe to render at all (internal/prompt's
// `drawing` layer is what asks for them). Nothing here has to allow it — the
// point of the note is that nothing may quietly forbid it either.
export function renderMarkdown(text: string): string {
  return DOMPurify.sanitize(marked.parse(text, { async: false }) as string)
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
