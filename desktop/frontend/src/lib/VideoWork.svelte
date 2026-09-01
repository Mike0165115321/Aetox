<script lang="ts">
  // งานวิดีโอ: the room that asks the one question this work starts with —
  // are we making a video, or cutting one that already exists?
  //
  // **Why this is a room when ระบบออโตเมชั่น stopped being one.** That button
  // called `newChairSession('automation')`, the identical line the roster's own
  // "แชทกับเอเจนนี้" calls, so it was not a room at all — it was a shortcut to
  // one agent, and if one agent earns a nav row for being important then every
  // agent can make the same argument and the roster has no reason to exist
  // (desks.ts). The rule that removed it is about *an agent* getting a row.
  //
  // This room is a subject with two agents behind it, and the choice between
  // them is the thing a person actually has in mind before anything else: the
  // material either exists or it does not, and nothing about the two jobs is
  // the same after that. A roster card cannot ask that question — it can only
  // list two names and leave the reader to work out which of them they are.
  //
  // Two severities on the card, deliberately different shapes:
  //
  //   - **AgentLock**, a veil over the whole card, when the editor these agents
  //     run on is not there at all. Nothing behind the card works, so nothing
  //     behind it is offered. Shared with every other roster (owner, 30 ส.ค.:
  //     *"ทำแบบนี้กับเอเจนทุกตัวนะครับ"*).
  //     The editor being installed and connected is not enough: kinocut answers
  //     a handshake and then refuses every encode with no ffmpeg on the machine,
  //     which is the same locked state by a different route. App.AgentGate
  //     answers both, and says which.
  import { onMount } from 'svelte'
  import { AgentGate, VideoCheckSeen } from '../../wailsjs/go/main/App'
  import { main } from '../../wailsjs/go/models'
  import { newChairSession, setActiveView } from './stores/cockpit.svelte'
  import { t, type TKey } from './i18n.svelte'
  import Icon from './Icon.svelte'
  import AgentLock from './AgentLock.svelte'
  import AgentFace from './AgentFace.svelte'
  import VideoReady from './VideoReady.svelte'
  import { coverHue } from './coverHue'
  import type { IconName } from './icons'

  let { onClose }: { onClose: () => void } = $props()

  // One entry per door, in the order the question is asked. The agent name is
  // the address, never the headline: `video` and `editor` are what the engine
  // calls them, and what a person picks between is making and cutting.
  const doors: { agent: string; icon: IconName; title: TKey; desc: TKey }[] = [
    { agent: 'video', icon: 'clapperboard', title: 'videowork.make', desc: 'videowork.makeDesc' },
    { agent: 'editor', icon: 'slidersHorizontal', title: 'videowork.cut', desc: 'videowork.cutDesc' },
  ]

  // One question per card, asked of one place, so the class on the card and the
  // sentence on the veil cannot disagree.
  let gates = $state<Record<string, main.AgentGate>>({})
  // Nothing is drawn until the verdict is in. A card that appears usable and is
  // veiled a moment later has already told the reader something untrue, and it
  // is the one failure a lock cannot have — the empty moment is shorter than
  // the wrong one, and the answer is cached after the first ask.
  let ready = $state(false)
  // The readiness panel: opened by itself the first time somebody walks in, and
  // a button after that. Owner, 30 ส.ค.: *"แสดงแค่ครั้งแรก"* — the machine is
  // worth an account of once, and nagging about it every visit is how a useful
  // screen becomes furniture.
  // Which agent's readiness is on screen, empty when the panel is closed. The
  // first-visit opening uses the scene-rendering agent because its list is the
  // superset; pressing a card's lock asks about that card.
  let checkFor = $state('')

  async function refresh() {
    const answers = await Promise.all(doors.map((d) => AgentGate(d.agent)))
    gates = Object.fromEntries(doors.map((d, i) => [d.agent, answers[i]]))
    ready = true
  }

  onMount(async () => {
    await refresh()
    if (!(await VideoCheckSeen())) checkFor = 'video'
  })

  // The view moves first, then the session boots: a click that waits for a
  // bootstrap before showing anything reads as a dead click (Office.svelte).
  async function start(agent: string) {
    setActiveView('chat')
    await newChairSession(agent)
  }

</script>

<div class="page-shell">
  <header class="page-head">
    <button class="settings-back" onclick={onClose}><Icon name="arrowLeft" size={14} /> {t('settings.backToApp')}</button>
    <div class="page-title">
      <!-- The badge covers the whole subject in one word, so it sits on the
           heading and nowhere else: repeating it on both cards would say the
           same thing twice, and the chat header is a surface this room's
           reader has already passed through. What "experimental" MEANS is one
           sentence in the footer note below, where the reading happens. -->
      <h2>{t('desk.videowork')} <span class="beta-chip">{t('videowork.beta')}</span></h2>
      <p>{t('videowork.intro')}</p>
    </div>
  </header>

  <div class="page-body">
    <div class="settings-inner">
      <!-- Two cards and nothing else above the fold. The roster's card is
           borrowed whole rather than restyled: this is the same kind of thing
           being chosen in the same app, and a second visual language for it
           would say the two rooms are unrelated. -->
      <div class="office-grid">
        {#each ready ? doors : [] as d (d.agent)}
          {@const locked = gates[d.agent]?.blocked ?? false}
          <div class="chair-card" class:locked>
            <span class="chair-band" style="--h:{coverHue(d.agent)}"></span>
            <div class="chair-body">
              <div class="chair-who">
                <!-- The person, not their glyph. The card was already borrowed
                     whole from the roster on the reasoning that this is the same
                     kind of thing being chosen in the same app; a face here and
                     a face there is the rest of that sentence. -->
                <AgentFace name={d.agent} icon={d.icon} size={38} />
                <span class="chair-name">{t(d.title)}</span>
              </div>
              <p class="chair-desc">{t(d.desc)}</p>
              <div class="chair-foot">
                <button class="chair-talk" onclick={() => start(d.agent)}>
                  <Icon name="messageSquare" size={14} />
                  <span class="t">{t('videowork.start')}</span>
                </button>
              </div>
            </div>
            <AgentLock agent={d.agent} label={t(d.title)} gate={gates[d.agent] ?? null}
              onPress={() => (checkFor = d.agent)} onInstalled={refresh} />
          </div>
        {/each}
      </div>

      <p class="office-note foot">{t('videowork.note')}</p>
      <!-- Said on the screen and not only in THIRD-PARTY-NOTICES, because the
           engine is somebody else's work and this is where a person is standing
           when they use it. Owner, 30 ส.ค.: *"บอกด้วยว่าสร้างวิดีโอด้วยเทคโนโลยี
           hyperframes"*. It names the making side alone: cutting runs on a
           different program, and crediting this one there would be false. -->
      <p class="office-note foot">{t('videowork.betaNote')} {t('videowork.engine')}</p>
    </div>
  </div>
</div>

{#if checkFor}
  <VideoReady agent={checkFor} onClose={() => { checkFor = ''; void refresh() }} />
{/if}
