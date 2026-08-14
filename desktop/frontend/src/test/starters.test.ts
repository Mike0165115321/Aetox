// The first sentence a room says (starters.ts).
//
// One card set for the whole app meant ผู้ช่วย opened with "รีวิวโค้ดและแนะนำ
// การแก้ไข" — a desk with no code tools offering code work — and a chat with the
// document agent opened with the same four. So what is pinned here is not that
// four buttons render: it is that the cards follow the SESSION, and in the
// session's own order of specificity (chair, then project, then desk), because
// that is the order the window itself resolves a room in.
//
// The second half is the line between the window and a worker's folder. An
// agent's opening is NOT in this app — it is a file in the agent's package, and
// the window asks for it (ChairStarters). What is pinned here is the seam: the
// answer is used when there is one, the generic four stand when there is not,
// and the window never trusts the file blindly.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import Chat from '../lib/Chat.svelte'
import { ChairStarters } from './mocks/wailsApp'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { startersFor, dealStarters, STARTER_SLOTS } from '../lib/starters'
import { th } from '../lib/locales/th'
import { en } from '../lib/locales/en'

const chatProps = {
  task: { title: '', steps: [] } as any,
  awaitingReply: false,
  agentStatus: '',
  toolSteps: [] as any[],
  streamingText: '',
  reasoningText: '',
  messages: [] as any[],
  onSend: () => {},
  onSwitchProvider: async () => {},
  onSwitchThinkLevel: async () => {},
  onSwitchModel: async () => {},
  onSubmitAPIKey: async () => {},
  model: { provider: 'deepseek', modelName: 'v4', thinkLevel: 'high', approval: 'ask', wireFormat: '' } as any,
}

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.desk = ''
  cockpit.chair = ''
  cockpit.space = ''
  cockpit.activeView = 'chat'
  cockpit.chat.length = 0
})

describe('which room the empty chat speaks for', () => {
  it('is the desk for an ordinary chat', () => {
    expect(startersFor({ desk: 'coding', chair: '', space: '' }).headlineKey).toBe('start.coding.headline')
    expect(startersFor({ desk: 'assistant', chair: '', space: '' }).headlineKey).toBe('start.assistant.headline')
  })

  // A project chat runs at the assistant's desk — that is where its tools come
  // from — so reading the desk alone would put the machine's cards on a chat
  // whose whole point is the folder of files riding into it.
  it('is the project, not the desk it runs at', () => {
    expect(startersFor({ desk: 'assistant', chair: '', space: 'เปิดร้านกาแฟ' }).headlineKey)
      .toBe('start.project.headline')
  })

  // A chair beats both — but the window does not hold a card set per agent, and
  // must not start holding one again: an agent's opening belongs to its folder,
  // and every agent looks the same from here, shipped or hired this morning.
  it('is the same generic colleague floor for every agent, named or not', () => {
    const first = startersFor({ desk: 'specialized', chair: 'automation', space: '' })
    for (const chair of ['automation', 'sheet', 'accounting-nobody-shipped']) {
      const set = startersFor({ desk: 'specialized', chair, space: '' })
      expect(set.headlineKey).toBe('start.chair.headline')
      // The same floor, not merely the same size — a set that varied by name
      // would be the window holding a card list per agent again.
      expect(set.starters, `${chair} got its own floor`).toEqual(first.starters)
    }
  })

  // '' is every session from before desks existed, plus every moment before the
  // engine has answered which desk this one is at.
  it('falls back to ผู้ช่วย when the desk is not known yet', () => {
    expect(startersFor({ desk: '', chair: '', space: '' }).headlineKey).toBe('start.assistant.headline')
  })
})

describe('every set the window owns', () => {
  const rooms = [
    { desk: 'assistant', chair: '', space: '' },
    { desk: 'coding', chair: '', space: '' },
    { desk: 'assistant', chair: '', space: 'p' },
    { desk: 'specialized', chair: 'any-agent', space: '' },
  ]

  // A set now holds a POOL and the grid draws four of it (dealStarters), so
  // what a set owes is no longer an exact count — it is enough cards to fill
  // the grid. A room holding three would deal a widow onto the second row.
  it('holds at least a full hand', () => {
    for (const room of rooms) {
      const n = startersFor(room).starters.length
      expect(n, `${room.desk}/${room.chair}/${room.space}: ${n} cards`).toBeGreaterThanOrEqual(STARTER_SLOTS)
    }
  })

  // th.ts is the source of truth for keys, so a missing Thai string is a
  // compile error — but a missing ENGLISH one is silent, and falls back to Thai
  // in front of an English-speaking user.
  it('is written in both languages', () => {
    for (const room of rooms) {
      const set = startersFor(room)
      for (const key of [set.headlineKey, ...set.starters.flatMap((s) => [s.titleKey, s.promptKey])]) {
        expect(th[key], `th: ${key}`).toBeTruthy()
        expect(en[key], `en: ${key}`).toBeTruthy()
      }
    }
  })
})

// A pool exists so a room can keep growing without the screen growing with it.
// What that costs is a promise: the cards you have not seen have to actually
// come round, or a pool of a hundred is a pool of four with ninety-six pieces
// of dead weight behind it.
describe('dealing four out of a pool', () => {
  const pool = Array.from({ length: 10 }, (_, i) => `card-${i}`)

  it('fills the grid and never puts the same card on screen twice', () => {
    for (let n = 0; n < 30; n++) {
      const hand = dealStarters('t-hand', pool, (c) => c)
      expect(hand).toHaveLength(STARTER_SLOTS)
      expect(new Set(hand).size).toBe(STARTER_SLOTS)
    }
  })

  // The reason this is a deck and not a dice roll. Rolling four at random can
  // hide a card for a dozen openings; the pool is only worth holding if what is
  // in it surfaces.
  it('deals every card in the pool before any card comes back', () => {
    const seen = new Set<string>()
    // Ten cards, four to a hand — three hands cover the pool with two to spare.
    for (let n = 0; n < 3; n++) for (const c of dealStarters('t-cover', pool, (c) => c)) seen.add(c)
    expect(seen.size).toBe(pool.length)
  })

  // The ordinary case for an agent: a folder holding exactly the four cards the
  // grid draws. Shuffling those would reorder the author's list for no reason.
  it('hands back a pool no bigger than a hand, in the order it was written', () => {
    const four = pool.slice(0, STARTER_SLOTS)
    expect(dealStarters('t-small', four, (c) => c)).toEqual(four)
  })

  // Bags are per room. A chat opened on the workshop desk must not decide what
  // ผู้ช่วย shows you next.
  it('keeps a separate bag per room', () => {
    const a = new Set<string>()
    for (let n = 0; n < 3; n++) for (const c of dealStarters('t-room-a', pool, (c) => c)) a.add(c)
    const b = new Set<string>()
    for (let n = 0; n < 3; n++) for (const c of dealStarters('t-room-b', pool, (c) => c)) b.add(c)
    expect(a.size).toBe(pool.length)
    expect(b.size).toBe(pool.length)
  })
})

describe('the empty chat on screen', () => {
  // Which four is a deal, so naming one of them here would be a test that fails
  // on a shuffle. What is pinned is the seam that matters: the hand fills the
  // grid, and every card in it came out of THIS room's pool.
  it('shows the desk’s own question and a hand out of its own pool', async () => {
    cockpit.desk = 'assistant'
    const pool = startersFor({ desk: 'assistant', chair: '', space: '' })
      .starters.map((s) => th[s.titleKey])

    const { container } = render(Chat, chatProps)

    await waitFor(() => {
      expect(screen.getByText(th['start.assistant.headline'])).toBeTruthy()
      const shown = [...container.querySelectorAll('.starter-card .title')].map((n) => n.textContent)
      expect(shown).toHaveLength(STARTER_SLOTS)
      for (const title of shown) expect(pool, `dealt a card no room owns: ${title}`).toContain(title)
      // and not the workshop's, which is what every room used to show
      expect(screen.queryByText(th['start.coding.reviewTitle'])).toBeNull()
    })
  })

  // Six cards behind four slots is the whole point of the pool, so the way to
  // reach the other two has to exist. A room whose pool is exactly a hand does
  // not get the button — it would deal the same four back.
  it('offers another hand only when there is one behind the four', async () => {
    cockpit.desk = 'assistant'
    const { container } = render(Chat, chatProps)
    await waitFor(() => expect(container.querySelector('.starter-more')).toBeTruthy())
  })

  // The seam. The window asks the agent's folder and draws what comes back —
  // and this is what a worker the app has never heard of relies on, so it is
  // checked with a name that is not one of the shipped five.
  it('lets an agent open with its own question and cards', async () => {
    vi.mocked(ChairStarters).mockResolvedValue({
      headline: 'ปิดบัญชีเดือนนี้หรือยัง?',
      cards: [{ title: 'กระทบยอดธนาคาร', prompt: 'ช่วยกระทบยอดธนาคารเดือนนี้: ', icon: 'chartColumn' }],
    })
    cockpit.desk = 'specialized'
    cockpit.chair = 'accounting'

    render(Chat, chatProps)

    await waitFor(() => {
      expect(ChairStarters).toHaveBeenCalledWith('accounting', 'th')
      expect(screen.getByText('ปิดบัญชีเดือนนี้หรือยัง?')).toBeTruthy()
      expect(screen.getByText('กระทบยอดธนาคาร')).toBeTruthy()
      // its cards REPLACE the generic ones rather than joining them
      expect(screen.queryByText(th['start.chair.whatTitle'])).toBeNull()
    })
  })

  // A worker with no opening of its own is the ordinary case, not a blank
  // screen: an agent is a folder, and most folders will never hold this file.
  it('falls back to the generic floor when the agent keeps no opening', async () => {
    cockpit.desk = 'specialized'
    cockpit.chair = 'accounting'
    const floor = startersFor({ desk: 'specialized', chair: 'accounting', space: '' })
      .starters.map((s) => th[s.titleKey])

    const { container } = render(Chat, chatProps)

    await waitFor(() => {
      expect(screen.getByText(th['start.chair.headline'])).toBeTruthy()
      const shown = [...container.querySelectorAll('.starter-card .title')].map((n) => n.textContent)
      expect(shown).toHaveLength(STARTER_SLOTS)
      for (const title of shown) expect(floor, `not from the generic floor: ${title}`).toContain(title)
    })
  })

  // The file is hand-written by someone who cannot see this build's icon set.
  // An unknown name would draw an empty box where every other card has a mark.
  it('does not draw an icon the file made up', async () => {
    vi.mocked(ChairStarters).mockResolvedValue({
      headline: '',
      cards: [{ title: 'การ์ดไอคอนมั่ว', prompt: 'ทำอะไรสักอย่าง', icon: 'no-such-icon' }],
    })
    cockpit.desk = 'specialized'
    cockpit.chair = 'accounting'

    const { container } = render(Chat, chatProps)

    await waitFor(() => expect(screen.getByText('การ์ดไอคอนมั่ว')).toBeTruthy())
    const glyph = container.querySelector('.starter-card .ic svg')
    expect(glyph?.innerHTML.trim()).toBeTruthy()
  })
})
