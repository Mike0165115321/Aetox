<script lang="ts">
  /**
   * One thing the agent asked to remember, offered under the answer that asked.
   *
   * Memory is the only tool whose work does not happen when it runs: it queues a
   * proposal and waits for a person (internal/learned/tool.go). Before this card
   * the whole of that showed up in the chat as a row reading "memory" inside a
   * collapsed panel, and the decision lived on the Settings page — so the user
   * saw neither what was proposed nor that anything was waiting, unless they
   * went looking. The work suggested it here; it is decided here.
   *
   * The row is read from the queue by id rather than carried in the transcript,
   * so a proposal decided anywhere else comes back saying which way it went
   * instead of asking a second time.
   *
   * Two shapes, because it is two different things (owner's pick, 16 ส.ค.):
   *
   *  - **Waiting** is a question, and gets a card. Open, always: the first cut
   *    collapsed it behind a chevron, which asked the user to click before they
   *    could see what they were being asked. Nothing is hidden, so there is no
   *    chevron either.
   *  - **Decided** is history, and gets one quiet line. A conversation that
   *    remembered five things would otherwise be a wall of cards, each one
   *    asking about a decision already made.
   */
  import Icon from './Icon.svelte'
  import { t } from './i18n.svelte'
  import { cockpit } from './stores/cockpit.svelte'
  import { scopeLabel } from './memoryScope'
  import { PendingChangeByID, ApprovePendingChange, RejectPendingChange } from '../../wailsjs/go/main/App'
  import type { main } from '../../wailsjs/go/models'

  let { id }: { id: number } = $props()

  let change = $state<main.PendingChange | null>(null)
  /** Only ever the decided line's disclosure — a waiting card has nothing shut. */
  let detail = $state(false)
  let busy = $state(false)
  let error = $state('')

  async function load(): Promise<void> {
    try {
      const row = await PendingChangeByID(id)
      // The queue is the truth: a row that is no longer there (the history was
      // cleared, the database was moved) draws nothing at all, rather than a
      // card offering a decision on something that cannot be decided.
      change = row?.id ? row : null
    } catch {
      change = null
    }
  }

  // Loads on mount, and again whenever the badge moves: any decision anywhere
  // emits learning:changed, and this card may be about the row that moved it.
  // Cheaper than an event listener per card, and it covers the case the card
  // cannot see for itself — the same proposal approved from Settings.
  $effect(() => {
    void cockpit.pendingLearned
    void load()
  })

  async function decide(approve: boolean): Promise<void> {
    if (busy) return
    busy = true
    error = ''
    try {
      if (approve) await ApprovePendingChange(id)
      else await RejectPendingChange(id)
      await load()
    } catch (err) {
      // Shown, not swallowed. An approval that could not be applied leaves the
      // proposal exactly where it was, and a button that seems to do nothing is
      // how a person decides the feature is broken.
      error = String(err)
    } finally {
      busy = false
    }
  }

  const pending = $derived(change?.state === 'pending')
  const scope = $derived(scopeLabel(change?.scope ?? ''))
  // The verb goes in the heading, which is why no `op` badge is drawn: "ADD"
  // was the database's own enum, in English, in the middle of a Thai sentence.
  // The one operation the heading cannot fully carry is a replacement, and that
  // one shows the line it overwrites instead.
  const asking = $derived(
    change?.op === 'remove' ? t('chat.memoryForgetAsk')
      : change?.op === 'replace' ? t('chat.memoryReplaceAsk')
      : t('chat.memoryAsk'),
  )
  const askIcon = $derived(
    change?.op === 'remove' ? 'x' : change?.op === 'replace' ? 'refreshCw' : 'brain',
  )
  const settled = $derived(change?.state === 'approved' ? t('chat.memoryKept') : t('chat.memoryDropped'))
  // A removal has no new text, so the line it is about is the one to show.
  const line = $derived(change?.body || change?.before || '')
</script>

{#if change && pending}
  <div class="memcard">
    <div class="memcard-head">
      <span class="ic"><Icon name={askIcon} size={15} /></span>
      <span class="memcard-kind">{asking}</span>
      <!-- Whose memory this lands in, at the top right, because it is the
           question being decided: a line true everywhere costs every session,
           and one true only here costs nothing anywhere else. -->
      <span class="memcard-scope">{scope}</span>
    </div>
    {#if change.before}
      <!-- What it overwrites, struck through above what it becomes: approving a
           change without seeing what it replaces is not a decision. -->
      <div class="memcard-was">{change.before}</div>
    {/if}
    {#if change.body}<div class="memcard-line">{change.body}</div>{/if}
    {#if change.reason}
      <div class="memcard-why">{t('chat.memoryBecause')} {change.reason}</div>
    {/if}
    {#if error}<div class="memcard-error">{error}</div>{/if}
    <div class="memcard-foot">
      <!-- Its own room in the footer rather than crammed beside the buttons:
           it is the one thing about this card a person assumes wrongly. -->
      <span class="memcard-note">{t('chat.memoryNextChat')}</span>
      <button type="button" class="memcard-no" disabled={busy}
        onclick={() => decide(false)}>{t('settings.learningReject')}</button>
      <button type="button" class="memcard-yes" disabled={busy}
        onclick={() => decide(true)}>{t('settings.learningApprove')}</button>
    </div>
  </div>
{:else if change}
  <div class="memdone" class:open={detail}>
    <button type="button" class="memdone-head" onclick={() => (detail = !detail)} aria-expanded={detail}>
      <span class="ic"><Icon name={change.state === 'approved' ? 'check' : 'x'} size={13} /></span>
      <span class="memdone-what">{settled}</span>
      <span class="memdone-scope">{scope}</span>
      <span class="chev"><Icon name={detail ? 'chevronDown' : 'chevronRight'} size={12} /></span>
    </button>
    {#if detail}
      <div class="memdone-body">
        {#if change.before}<div class="memcard-was">{change.before}</div>{/if}
        {#if line}<div class="memdone-line">{line}</div>{/if}
        {#if change.reason}
          <div class="memcard-why">{t('chat.memoryBecause')} {change.reason}</div>
        {/if}
      </div>
    {/if}
  </div>
{/if}
