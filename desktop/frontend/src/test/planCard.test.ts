// A plan in an answer (the วางแผน stance, §106.11), from the outside.
//
// The stance decides that a turn produces a plan and pins its four headings;
// internal/prompt's `planCard` layer asks the model to wrap that plan in a
// ```plan fence, but only on a surface that can draw one. This is the other end
// of that contract — the fence arriving and becoming a card.
//
// The failure this guards against is silent in the worst way: the model does
// exactly what it was told, the fence is not recognised, and the user is handed
// their plan as a grey block of monospaced source.
import { describe, it, expect } from 'vitest'
import { renderMarkdown, renderStreamingMarkdown } from '../lib/markdown'

const plan = `\`\`\`plan
# แผนปรับ Backend ให้ประหยัด DB Connection

**What is there now** — the pool is opened per request in \`db.go\`.

**What to change** — hold one pool on the app.

**What could go wrong** — nothing else reads the pool.

**What you are unsure of** — whether Railway caps connections.
\`\`\``

// The card carries the model's raw markdown on data-plan so Copy can hand it
// back (see the copy test below), which means the source appears twice in the
// output — once as an attribute, once drawn. Assertions about what the user
// *sees* have to look past the attribute or they pass on the copy of the plan
// nobody is reading.
const drawn = (html: string) => html.replace(/ data-plan="[^"]*"/, '')

describe('a plan in an answer', () => {
  it('becomes a card, not a code block', () => {
    const out = renderMarkdown(plan)

    expect(out).toContain('plan-card')
    // The whole point. A plan rendered as source is the bug this replaced.
    expect(out).not.toContain('codeblock')
    expect(out).not.toContain('hljs')
  })

  it('lifts the first heading out as the card title', () => {
    const out = renderMarkdown(plan)

    expect(out).toContain('plan-title')
    expect(out).toContain('แผนปรับ Backend')
    // Lifted, not copied: a title drawn above a body that still opens with the
    // same line is the plan announcing itself twice.
    expect(drawn(out).match(/แผนปรับ Backend/g)?.length).toBe(1)
  })

  it('renders the plan as markdown rather than as text', () => {
    const out = renderMarkdown(plan)

    for (const heading of [
      'What is there now',
      'What to change',
      'What could go wrong',
      'What you are unsure of',
    ]) {
      expect(out).toContain(heading)
    }
    // The headings are bold in the source; if the body were inserted as text
    // the user would be reading asterisks.
    expect(out).toContain('<strong>')
    expect(drawn(out)).not.toContain('**What to change**')
    // Inline code survives, which is what the prompt tells the model to use for
    // a filename instead of a fence it cannot open inside the card.
    expect(out).toContain('<code>db.go</code>')
  })

  // Same guarantee the drawing and the panel have. marked treats an unclosed
  // fence as running to the end of the text, so the card exists from the moment
  // the model writes the opening fence and fills in as the plan arrives —
  // rather than sitting as raw source until the very last token closes it.
  it('builds itself as it streams', () => {
    const out = renderStreamingMarkdown(plan.slice(0, 120))

    expect(out).toContain('plan-card')
    expect(out).not.toContain('```')
  })

  // Copy has to hand back what the model wrote. Read off the rendered card
  // instead, it would return the plan with every heading and bullet flattened
  // out — no longer something to paste into an issue.
  it('carries its own markdown for the copy button', () => {
    const out = renderMarkdown(plan)

    expect(out).toContain('data-plan=')
    expect(out).toContain('plan-copy')
    expect(out).toContain('**What to change**')
  })

  // The card's icon is the app's furniture, not something the user can take
  // away. Without the data-chrome guard in confine() it is framed as a drawing,
  // with copy and save buttons hung off a 14px glyph.
  it('does not frame its own icon as a drawing', () => {
    const out = renderMarkdown(plan)

    expect(out).toContain('<svg')
    expect(out).not.toContain('drawing-box')
    expect(out).not.toContain('drawing-save')
  })

  // A plan with no heading is a card with no title rather than a card that
  // swallowed its first sentence.
  it('survives a plan that opens with prose', () => {
    const out = renderMarkdown('```plan\nJust one line, and nothing to change.\n```')

    expect(out).toContain('plan-card')
    expect(out).not.toContain('plan-title')
    expect(out).toContain('Just one line')
  })

  // A ```plan written inside a plan is a model showing the fence rather than
  // opening a second card. Without the guard this recurses.
  it('does not nest a plan inside a plan', () => {
    const out = renderMarkdown('```plan\n# T\n\n````plan\ninner\n````\n```')

    expect(out.match(/plan-card/g)?.length).toBe(1)
  })
})
