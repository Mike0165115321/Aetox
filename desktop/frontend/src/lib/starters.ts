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
// The card fills the composer rather than sending — the prompts ending in ": "
// are deliberately unfinished, because the half a sentence only the user can
// write is exactly the half worth asking for.

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
  starters: Starter[]
}

/** Keyed by context id: a desk name, or 'project'. `chair` is what any office
 *  agent opens with when its own folder holds no STARTERS.md. */
const SETS: Record<string, StarterSet> = {
  assistant: {
    headlineKey: 'start.assistant.headline',
    starters: [
      { icon: 'folderOpen', titleKey: 'start.assistant.findTitle', promptKey: 'start.assistant.findPrompt' },
      { icon: 'fileText', titleKey: 'start.assistant.readTitle', promptKey: 'start.assistant.readPrompt' },
      { icon: 'globe', titleKey: 'start.assistant.webTitle', promptKey: 'start.assistant.webPrompt' },
      { icon: 'bot', titleKey: 'start.assistant.makeTitle', promptKey: 'start.assistant.makePrompt' },
    ],
  },

  coding: {
    headlineKey: 'start.coding.headline',
    starters: [
      { icon: 'compass', titleKey: 'start.coding.exploreTitle', promptKey: 'start.coding.explorePrompt' },
      { icon: 'wrench', titleKey: 'start.coding.buildTitle', promptKey: 'start.coding.buildPrompt' },
      { icon: 'search', titleKey: 'start.coding.reviewTitle', promptKey: 'start.coding.reviewPrompt' },
      { icon: 'bandage', titleKey: 'start.coding.fixTitle', promptKey: 'start.coding.fixPrompt' },
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
