// The four cards on an empty chat, for the rooms that are part of the product.
//
// One list of starters for the whole app was a card set written for the
// workshop shown in every other room: "รีวิวโค้ดและแนะนำการแก้ไข" on ผู้ช่วย,
// which has no code tools, and on a chat with the document agent, who would not
// know what to do with it. The empty state is the first sentence a room says,
// so it has to be that room's sentence.
//
// **The agents' own cards are NOT here.** A worker keeps its opening in its own
// folder, beside its AGENT.md (internal/subagent/starters.go), and this file
// never learns their names. That line is the whole design: an agent is a folder
// the user can add, so a card set that lived here could only ever be given to
// the workers we shipped — hiring a bookkeeper would mean editing the product
// to let it say hello (owner's call, 2026-08-10). What is left here is the
// rooms the owner draws: the two desks, a project, and the four cards any
// colleague opens with when its folder holds no opening of its own.
//
// The rule these sets are written under is the same one the desks are under: a
// card may only ask for work the session on the other side can actually do
// (docs/DECISIONS.md — "โต๊ะที่ไม่มีเครื่องมือ ต้องไม่ถูกสั่งให้ใช้มัน"). A
// starter is a pre-filled request, so an impossible one costs the user a turn
// and reads as the assistant's failure rather than the card's.
//
// **And "can do" is not the bar — "is good at" is** (owner's call, 2026-08-15).
// A trip-planning card shipped here briefly and was cut, not because the tools
// were missing on paper — it is web search and a written file, both present —
// but because a real trip plan turns on distances, opening hours and a map, and
// this product has no sense of place at all. Every tool the card touched
// existed; the thing it promised was still something we are bad at. So the test
// before a card goes in the pool is not "does a tool exist for each step", it
// is "would we be pleased to be judged on this" — the empty screen is where a
// user forms their opinion of what this app is for, and a card is a promise
// made before any work is done.
//
// The card fills the composer rather than sending — the prompts ending in ": "
// are deliberately unfinished, because the half a sentence only the user can
// write is exactly the half worth asking for.
//
// **A card is two pieces of writing with two different jobs** (owner's call,
// 2026-08-15). The title sells the outcome and is read in a second, so it stays
// short. The prompt is not a restatement of the title — it is the actual
// instruction the model receives, nobody has to read it, and every sentence
// added to it is quality in the work that comes back. So ผู้ช่วย's prompts are
// written the way a prompt engineer writes: they name the sources to use, the
// rule against inventing a number, the order of work, the exact artifact
// wanted, and what to say about where each finding came from. Writing them as
// one-line echoes of the title was the mistake this note exists to prevent.
//
// **ผู้ช่วย's cards name a real job, and are finished sentences** (owner's call,
// 2026-08-15). The four they replace named an ability and stopped — "อ่านและ
// สรุปเอกสารหรือสื่อ", "ค้นข้อมูลจากเว็บแล้วสรุปให้" — which is the sentence a
// chat tab in a browser also says, so the room that does the work on your
// machine was opening with the one claim that proves nothing. What replaced
// them was written under one rule: **a card ends in something you can open.**
// That is the line the owner drew when he kept two of a proposed six and cut
// the rest; the survivors both handed back a file, and every one he cut handed
// back a summary. It is also what this class of product actually says on its
// front page — Manus opens with "Create slides / Build website / Create games"
// and shows finished jobs with proper nouns ("Trip to Japan in april"), never
// "summarize a document".
//
// Naming a real subject rather than a shape ("โมเดล AI ตัวไหนคุ้มที่สุด", not
// "ค้นข้อมูลเรื่องหนึ่ง") costs the user a delete when they wanted a different
// subject, and buys the thing an empty screen is for: they see the whole job at
// once instead of decoding what this app is willing to be asked. The owner
// weighed that trade and took the picture.

import type { IconName } from './icons'
import type { TKey } from './i18n.svelte'

export interface Starter {
  icon: IconName
  titleKey: TKey
  promptKey: TKey
}

export interface StarterSet {
  /** The question above the cards. It belongs to the set: "จะให้เราสร้างอะไรดี"
   *  is the workshop asking, and ผู้ช่วย is not mainly asked to build. */
  headlineKey: TKey
  /** Every card the room could open with. NOT what is drawn — `dealStarters`
   *  hands the window four of these. A set may hold as many as it has. */
  starters: Starter[]
}

/** Keyed by context id: a desk name, or 'project'. `chair` is what any office
 *  agent opens with when its own folder holds no STARTERS.md. */
const SETS: Record<string, StarterSet> = {
  assistant: {
    headlineKey: 'start.assistant.headline',
    starters: [
      { icon: 'search', titleKey: 'start.assistant.chartTitle', promptKey: 'start.assistant.chartPrompt' },
      { icon: 'monitor', titleKey: 'start.assistant.healthTitle', promptKey: 'start.assistant.healthPrompt' },
      { icon: 'layoutList', titleKey: 'start.assistant.moneyTitle', promptKey: 'start.assistant.moneyPrompt' },
      { icon: 'terminal', titleKey: 'start.assistant.appTitle', promptKey: 'start.assistant.appPrompt' },
      { icon: 'globe', titleKey: 'start.assistant.scrapeTitle', promptKey: 'start.assistant.scrapePrompt' },
      { icon: 'brain', titleKey: 'start.assistant.learnTitle', promptKey: 'start.assistant.learnPrompt' },
      { icon: 'chartColumn', titleKey: 'start.assistant.bizTitle', promptKey: 'start.assistant.bizPrompt' },
      { icon: 'timer', titleKey: 'start.assistant.runwayTitle', promptKey: 'start.assistant.runwayPrompt' },
      { icon: 'compass', titleKey: 'start.assistant.marketTitle', promptKey: 'start.assistant.marketPrompt' },
      { icon: 'package', titleKey: 'start.assistant.deckTitle', promptKey: 'start.assistant.deckPrompt' },
    ],
  },

  coding: {
    headlineKey: 'start.coding.headline',
    starters: [
      { icon: 'bandage', titleKey: 'start.coding.fixTitle', promptKey: 'start.coding.fixPrompt' },
      { icon: 'wrench', titleKey: 'start.coding.buildTitle', promptKey: 'start.coding.buildPrompt' },
      { icon: 'compass', titleKey: 'start.coding.exploreTitle', promptKey: 'start.coding.explorePrompt' },
      { icon: 'search', titleKey: 'start.coding.reviewTitle', promptKey: 'start.coding.reviewPrompt' },
      { icon: 'package', titleKey: 'start.coding.upgradeTitle', promptKey: 'start.coding.upgradePrompt' },
      { icon: 'shield', titleKey: 'start.coding.testTitle', promptKey: 'start.coding.testPrompt' },
      { icon: 'timer', titleKey: 'start.coding.perfTitle', promptKey: 'start.coding.perfPrompt' },
      { icon: 'puzzle', titleKey: 'start.coding.refactorTitle', promptKey: 'start.coding.refactorPrompt' },
    ],
  },

  // A chat held inside a โปรเจกต์. What makes it different from a plain
  // assistant chat is exactly one thing — the context folder rides into every
  // session in it — so all four cards are about that folder. Nothing here may
  // refer to the project's *other* chats: a session is handed the files, not
  // the earlier conversations, and a card promising a recap would be asking for
  // something the session cannot see.
  project: {
    headlineKey: 'start.project.headline',
    starters: [
      { icon: 'folder', titleKey: 'start.project.filesTitle', promptKey: 'start.project.filesPrompt' },
      { icon: 'search', titleKey: 'start.project.askTitle', promptKey: 'start.project.askPrompt' },
      { icon: 'wrench', titleKey: 'start.project.continueTitle', promptKey: 'start.project.continuePrompt' },
      { icon: 'layoutList', titleKey: 'start.project.planTitle', promptKey: 'start.project.planPrompt' },
      { icon: 'alertTriangle', titleKey: 'start.project.conflictTitle', promptKey: 'start.project.conflictPrompt' },
      { icon: 'fileText', titleKey: 'start.project.briefTitle', promptKey: 'start.project.briefPrompt' },
    ],
  },

  // Any office agent whose folder holds no STARTERS.md — the shipped five all
  // carry one, so in practice this is a worker the user hired, or one whose file
  // is empty. The four cards name the four ways a conversation with a colleague
  // starts, which stays true of a specialist whose craft this file has never
  // heard of. That is the only thing the window is allowed to assume about an
  // agent it did not ship.
  chair: {
    headlineKey: 'start.chair.headline',
    starters: [
      { icon: 'messageSquare', titleKey: 'start.chair.whatTitle', promptKey: 'start.chair.whatPrompt' },
      { icon: 'brain', titleKey: 'start.chair.consultTitle', promptKey: 'start.chair.consultPrompt' },
      { icon: 'folderOpen', titleKey: 'start.chair.lookTitle', promptKey: 'start.chair.lookPrompt' },
      { icon: 'zap', titleKey: 'start.chair.doTitle', promptKey: 'start.chair.doPrompt' },
      { icon: 'search', titleKey: 'start.chair.checkTitle', promptKey: 'start.chair.checkPrompt' },
      { icon: 'package', titleKey: 'start.chair.prepTitle', promptKey: 'start.chair.prepPrompt' },
    ],
  },
}

/** Which room the open session is in, most specific first.
 *
 * The order is the session's own order of specificity, not the nav's: a chair
 * session is with a named agent and that beats everything, a project chat is an
 * assistant chat filed somewhere so the folder wins over the desk, and the desk
 * answers the rest. The '' desk — sessions from before desks existed, and any
 * moment before the engine has answered — lands on ผู้ช่วย, which is the desk
 * those sessions behave like.
 *
 * For a chair this is the FLOOR, not the answer: the agent's own folder is
 * asked first (ChairStarters), and what comes back replaces this whenever the
 * worker had something to say. */
export function startersFor(ctx: { desk: string; chair: string; space: string }): StarterSet {
  if (ctx.chair) return SETS.chair
  if (ctx.space) return SETS.project
  return SETS[ctx.desk] ?? SETS.assistant
}

/** How many cards the grid draws, whatever the room is holding. */
export const STARTER_SLOTS = 4

/** Deal `count` cards out of a pool, dealing every card before repeating any.
 *
 * The empty state shows four cards and a room may hold far more — the point of
 * a pool is that a set can keep growing without the screen growing with it
 * (owner's call, 2026-08-15). Four was drawn from a fixed list before, which
 * meant a fifth card could only be added by taking one away.
 *
 * **A shuffled deck, not a dice roll.** Rolling four at random can hide a card
 * for a dozen openings and show the same hand twice in a row, which is exactly
 * backwards: the reason to hold a pool is that people find things in it. So a
 * bag is kept per room, dealt from, and re-shuffled only once it runs out —
 * every card is seen before any card comes back.
 *
 * The bag lives here rather than on disk. It is a nicety about what you saw
 * this afternoon, not a fact about the product, and losing it on restart costs
 * exactly one reshuffle. A room whose pool is no bigger than a hand skips all
 * of this and hands the pool straight back.
 */
// The bag holds NAMES, not cards. The pool is rebuilt from scratch every time
// the window re-derives it — new objects, same words — so a bag of cards would
// be a bag of stale references that match nothing in the pool it is supposed to
// be dealing from, and every "have I dealt this already" check would answer no.
// (It did. The grid drew the same card twice.) A name survives the rebuild, and
// dropping names the pool no longer has is also how a pool that CHANGED — the
// language switched, the agent's file arrived — heals itself instead of dealing
// cards that no longer exist.
const bags = new Map<string, string[]>()

export function dealStarters<T>(
  key: string,
  pool: readonly T[],
  nameOf: (card: T) => string,
  count = STARTER_SLOTS,
): T[] {
  if (pool.length <= count) return [...pool]

  const byName = new Map(pool.map((c) => [nameOf(c), c]))
  const hand: T[] = []
  const dealt = new Set<string>()
  let bag = (bags.get(key) ?? []).filter((n) => byName.has(n))

  while (hand.length < count) {
    // Re-shuffle excluding what is already in this hand, so a bag running dry
    // mid-deal cannot put the same card on screen twice.
    if (bag.length === 0) bag = shuffle([...byName.keys()].filter((n) => !dealt.has(n)))
    const name = bag.pop() as string
    if (dealt.has(name)) continue
    dealt.add(name)
    hand.push(byName.get(name) as T)
  }

  bags.set(key, bag)
  return hand
}

function shuffle<T>(items: readonly T[]): T[] {
  const out = [...items]
  for (let i = out.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1))
    ;[out[i], out[j]] = [out[j], out[i]]
  }
  return out
}
