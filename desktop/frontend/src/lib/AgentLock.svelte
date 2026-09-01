<script lang="ts">
  // The veil over an agent that cannot work.
  //
  // Owner, 30 ส.ค.: *"ทำ Ui ทับ เอเจนเลย เขียนว่าติดตั้งเครื่องมือไม่งั้น ใช้งานเอเจน
  // ไม่ได้ครับ"*, then *"ทำแบบนี้กับเอเจนทุกตัวนะครับ"* — so this is one component
  // used wherever an agent is offered, not something the video room grew for
  // itself.
  //
  // **It is drawn over the card rather than beside it** because the sentence is
  // about the whole card. A warning row under a live button says "there is a
  // problem and you may proceed"; this says "there is nothing to proceed to",
  // and those are different claims. The card underneath stays legible through
  // the veil so the reader still sees which teammate they are being told about.
  //
  // Three reasons, three sentences, three buttons — and `App.AgentGate` decides
  // which, so the roster, the video room and the chat menu cannot disagree
  // about the same agent:
  //
  //   - `install` — something Aetox fetches itself. The report opens first
  //     (ToolInstallReport), then the same InstallCapabilities the first-run
  //     screen calls, reporting itself in the strip over the window.
  //   - `connect` — an account or a service the user connects. Nothing is
  //     downloadable, so the button walks to the page whose one-click fix
  //     already exists. Offering to "install" a service would be a lie.
  //   - `incomplete` — the tool is installed and connected and says it cannot
  //     work: kinocut answers a handshake and then refuses every encode with no
  //     ffmpeg. That was drawn as a warning row until the owner saw it and said
  //     what it should have been: *"เครื่องมือไม่ครบ กูบอกให้ใช้งานไม่ได้ไง"*.
  import { InstallCapabilities, ToolInstallPlan } from '../../wailsjs/go/main/App'
  import { main } from '../../wailsjs/go/models'
  import { cockpit, setActiveView } from './stores/cockpit.svelte'
  import { capabilities, noteCapabilityRequest } from './capabilities.svelte'
  import { t, type TKey } from './i18n.svelte'
  import Icon from './Icon.svelte'
  import ToolInstallReport from './ToolInstallReport.svelte'

  let { agent, label, gate, onInstalled, onPress }: {
    agent: string
    /** What to call this agent on the veil. The room's own words where it has
     *  them ("สร้างวิดีโอใหม่"), the profile name everywhere else. */
    label: string
    gate: main.AgentGate | null
    onInstalled?: () => void
    /** A room with a better screen than the generic report takes the press
     *  itself — ห้องงานวิดีโอ opens its readiness panel, which answers the same
     *  question with more of the machine in it. */
    onPress?: () => void
  } = $props()

  let plan = $state<main.ToolInstallPlan | null>(null)
  const busy = $derived(capabilities.phase === 'installing')

  const SAY: Record<string, TKey> = {
    install: 'lock.body',
    connect: 'lock.bodyConnect',
    incomplete: 'lock.bodyIncomplete',
    // `enable` is `install` said from the reader's side. Same remedy, same one
    // press; what changes is that it talks about switching the teammate on
    // rather than about the parts being fetched, which are ours and not theirs.
    enable: 'lock.bodyEnable',
  }
  const DO: Record<string, TKey> = {
    install: 'lock.go',
    connect: 'lock.goConnect',
    incomplete: 'lock.go',
    enable: 'lock.goEnable',
  }
  const MARK: Record<string, 'wrench' | 'plug' | 'alertTriangle' | 'download'> = {
    install: 'wrench',
    connect: 'plug',
    incomplete: 'alertTriangle',
    enable: 'download',
  }

  // `kind` is optional on the wire (an unblocked gate has none), so it is
  // resolved once here rather than defaulted at each of the four places that
  // index a table with it.
  const kind = $derived(gate?.kind || 'install')

  // What is missing, named. A veil that says only "a tool is missing" sends the
  // reader to go and find out which; the answer is already in hand.
  const shortOf = $derived((gate?.missing ?? []).join(', '))

  // A gate carrying a capability opens Aetox's own install report, whatever its
  // kind: `install` and `incomplete` are two different sentences about the same
  // remedy, and both name things this app fetches. Sending somebody to a
  // download page for one of them was the wrong answer to a question we can
  // answer ourselves.
  async function press() {
    if (!gate) return
    if (onPress) {
      onPress()
      return
    }
    if (gate.capability) {
      const p = await ToolInstallPlan(gate.capability)
      if (p.parts.length > 0) {
        plan = p
        return
      }
      // Nothing to fetch after all — an unpinned entry, or a platform whose
      // manifest offers none of it. The settings page is where the rest of the
      // answers are; a download page is not one Aetox gets to give.
    }
    cockpit.settingsIntent = { section: 'team', agent }
    setActiveView('settings')
  }

  async function confirm() {
    const wanted = plan?.capability ?? ''
    plan = null
    noteCapabilityRequest([wanted])
    try {
      await InstallCapabilities([wanted])
    } catch {
      /* the strip over the window reports it */
    }
    onInstalled?.()
  }
</script>

{#if gate?.blocked}
  <!-- Opaque, not a wash. A translucent veil left the card's own title and
       description showing through underneath its message, and the two collided
       into something neither of them said. What stays visible is the coloured
       band at the top and the name repeated here, which is all the reader needs
       to know which teammate this is. -->
  <div class="chair-lock">
    <span class="chair-lock-who">{label}</span>
    <Icon name={MARK[kind] ?? 'wrench'} size={20} />
    <p class="chair-lock-say">{t(SAY[kind] ?? 'lock.body')}</p>
    {#if shortOf}<p class="chair-lock-what">{shortOf}</p>{/if}
    <button class="ctrl ctrl-primary" disabled={busy && !!gate.capability} onclick={press}>
      {busy && gate.capability ? t('lock.installing') : t(DO[kind] ?? 'lock.go')}
    </button>
  </div>
{/if}

{#if plan}
  <ToolInstallReport
    {plan}
    title={t('lock.reportTitle', { name: label })}
    onCancel={() => (plan = null)}
    onConfirm={confirm}
  />
{/if}
