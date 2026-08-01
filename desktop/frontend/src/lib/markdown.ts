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
export function renderMarkdown(text: string): string {
  return DOMPurify.sanitize(marked.parse(text, { async: false }) as string)
}
