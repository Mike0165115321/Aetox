<script lang="ts">
  // Work that outlives the turn that started it (§105), drawn as a card at the
  // foot of the conversation.
  //
  // A card rather than the status strip this replaced. The strip was accurate
  // and useless: the owner ran the first build, saw "ใช้ 1 เครื่องมือ · 1s"
  // frozen under a finished turn, and read it as dead work — "ถ้ามันทำงาน
  // ควรจะไม่นิ่งแบบนี้". A number that only changes every few seconds cannot
  // answer "is this alive"; a list of filenames scrolling past can, at a
  // glance, which is why the running card's centre of gravity is its last few
  // steps rather than its counters.
  //
  // Two sources, deliberately, joined on the delegation's id:
  //   - state (running/waiting/done, the counters, the question) from the
  //     ENGINE's register, which is the only thing that knows them — a `task`
  //     tool call completes the moment the work starts, so the event stream
  //     shows every delegation as finished from birth.
  //   - the step lines from the live event feed, because the register does not
  //     keep them and nothing else can make the card look alive.
  //
  // Below the transcript rather than inside it: the whole failure this fixes is
  // work you cannot see, and a card that scrolls away with the history is one
  // more way not to see it.
  import type { BackgroundTask, ToolStep } from './types'
  import { t } from './i18n.svelte'
  import Icon from './Icon.svelte'
  import { fold } from './fold'

  let {
    tasks, steps, onAnswer,
  }: {
    tasks: BackgroundTask[]
    steps: ToolStep[]
    /** Answering goes out as an ordinary chat message, the same door the user
     *  would type through — there is no second path into the engine. */
    onAnswer: (id: string, answer: string) => void
  } = $props()

  // A collected result is in the conversation now; the card has said all it can.
  const shown = $derived(tasks.filter((task) => !task.collected))

  // The clock ticks client-side off startedAt, so a running second costs no Go
  // call. Armed only while something is actually moving.
  let now = $state(Date.now())
  $effect(() => {
    if (!shown.some((task) => task.state === 'running' || task.state === 'waiting')) return
    const timer = setInterval(() => { now = Date.now() }, 1000)
    return () => clearInterval(timer)
  })

  function elapsed(startedAt: string): string {
    const secs = Math.max(0, Math.round((now - new Date(startedAt).getTime()) / 1000))
    const mins = Math.floor(secs / 60)
    return mins > 0 ? `${mins}m ${String(secs % 60).padStart(2, '0')}s` : `${secs}s`
  }

  // The last few steps of one delegation. Three because the point is movement,
  // not the record: forty rows would bury the conversation the card sits under,
  // and the whole log is already in the answer the delegate hands back.
  const STEP_TAIL = 3
  function tailOf(id: string): ToolStep[] {
    return steps.filter((s) => s.task === id && !s.kind).slice(-STEP_TAIL)
  }

  // One draft per waiting delegate, keyed by id: two of them parked at once
  // must not share a box.
  let drafts = $state<Record<string, string>>({})
  function send(id: string) {
    const answer = (drafts[id] ?? '').trim()
    if (!answer) return
    drafts[id] = ''
    onAnswer(id, answer)
  }
</script>

{#if shown.length > 0}
  <div class="bgw">
    {#each shown as task (task.id)}
      <!-- One wrapper per delegation, and the fold lives on it rather than on
           the three cards inside. A row entering or leaving the tray is a
           delegation starting or being collected, which is worth easing; the
           running→done swap within it is the same row changing its mind, and
           folding that would read as one card leaving and another arriving. -->
      <div class="bgw-item" transition:fold>
      {#if task.state === 'running'}
        <!-- The state class the turn timeline's card carries too: the running
             beam is styled on `.run`, and these two are deliberately one card
             (§105.5), so it cannot be a thing only one of them does. -->
        <div class="bgw-card run">
          <div class="bgw-head">
            <span class="bgw-mark run"><Icon name="loaderCircle" size={15} /></span>
            <b class="bgw-agent">{task.agent}</b>
            <span class="bgw-badge run">{t('bgw.running')}</span>
            <span class="bgw-meta">{elapsed(task.startedAt)} · {t('bgw.tools', { n: task.toolCalls })}</span>
          </div>
          <div class="bgw-brief">{task.label}</div>
          <div class="bgw-steps">
            {#each tailOf(task.id) as step (step.ref ?? step.label)}
              <!-- The same row the turn timeline draws (.tool-step), so a
                   delegate's step looks identical whether its turn is still
                   open or long finished. -->
              <div class="tool-step {step.state}">
                {#if step.state === 'run'}
                  <span class="glyph spin"></span>
                {:else}
                  <span class="glyph"><Icon name={step.state === 'done' ? 'check' : 'x'} size={12} /></span>
                {/if}
                <span class="lbl">{step.label}</span>
              </div>
            {/each}
          </div>
        </div>
      {:else if task.state === 'waiting'}
        <div class="bgw-card is-waiting">
          <div class="bgw-head">
            <span class="bgw-mark wait"><Icon name="hand" size={15} /></span>
            <b class="bgw-agent">{task.agent}</b>
            <span class="bgw-badge wait">{t('bgw.waiting')}</span>
            <span class="bgw-meta">{elapsed(task.startedAt)} · {t('bgw.tools', { n: task.toolCalls })}</span>
          </div>
          {#if task.question}<div class="bgw-question">{task.question}</div>{/if}
          <div class="bgw-answer">
            <input
              class="ctrl" type="text"
              placeholder={t('bgw.answerPlaceholder', { agent: task.agent })}
              bind:value={drafts[task.id]}
              onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); send(task.id) } }}
            />
            <button class="ctrl" onclick={() => send(task.id)}>{t('bgw.send')}</button>
          </div>
        </div>
      {:else}
        <!-- Finished, and nobody has to do anything about it: the moment the
             poll sees this state it sends the turn that collects the result
             (cockpit's autoCollectFinished), so this row is a receipt, not a
             control. It disappears when that turn redeems it. -->
        <div class="bgw-card is-done">
          <span class="bgw-mark {task.state === 'failed' ? 'fail' : 'ok'}">
            <Icon name={task.state === 'failed' ? 'x' : 'check'} size={15} />
          </span>
          <b class="bgw-agent">{task.agent}</b>
          <span class="bgw-done-note">
            {task.state === 'failed' ? t('bgw.failed') : t('bgw.finished')}
          </span>
          <span class="bgw-meta">{t('bgw.tools', { n: task.toolCalls })}</span>
        </div>
      {/if}
      </div>
    {/each}
  </div>
{/if}
