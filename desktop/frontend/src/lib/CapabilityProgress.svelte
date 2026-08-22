<script lang="ts">
  // The card that says a capability download is happening.
  //
  // Same shell as Updater.svelte, on purpose and not to save work: the two are
  // the same event to a user — something large is being fetched, they did not
  // have to wait for it, and they need to be told when it lands or fails. A
  // second card shaped differently would read as a second kind of thing.
  //
  // It appears without being asked for and leaves on its own once the work is
  // done, because the install was started on a screen the user has already left
  // and a receipt nobody dismissed is not a receipt.
  import { t } from './i18n.svelte'
  import Icon from './Icon.svelte'
  import { durationMs } from './motion'
  import {
    capabilities, dismissCapabilities, retryCapabilities,
  } from './capabilities.svelte'

  let hidden = $state(false)

  // The × hides what the card currently says, not the card forever — a failure
  // arriving after someone waved off a progress bar is news they have not had.
  let said = $state('')
  $effect(() => {
    if (capabilities.phase !== said) {
      said = capabilities.phase
      hidden = false
    }
  })

  // Success clears itself. Left up, a finished bar becomes another thing to
  // tidy away; taken down instantly, it reads as a crash rather than as done.
  $effect(() => {
    if (capabilities.phase !== 'done') return
    const timer = setTimeout(dismissCapabilities, durationMs('--dur-hold-done', 1600) * 2)
    return () => clearTimeout(timer)
  })

  const show = $derived(!hidden && capabilities.phase !== 'idle')
  const pct = $derived(capabilities.percent)

  const title = $derived(
    capabilities.phase === 'done'
      ? t('cap.added')
      : capabilities.phase === 'error'
        ? t('cap.failed')
        : t('cap.installing'),
  )
</script>

{#if show}
  <div class="upd-card" role="status" aria-live="polite">
    <div class="upd-head">
      <div class="upd-icon">
        <Icon name={capabilities.phase === 'done' ? 'check' : 'download'} size={20} />
      </div>
      <div class="upd-headings">
        <div class="upd-title">{title}</div>
        {#if capabilities.phase === 'installing' && capabilities.of > 0}
          <!-- Which download of how many, because one bar restarting three
               times with no count looks like one download going backwards. -->
          <div class="upd-date">
            {t('cap.progress', { done: String(capabilities.index + 1), total: String(capabilities.of) })}
          </div>
        {/if}
      </div>
      <button class="upd-x" aria-label={t('cap.hide')} onclick={() => (hidden = true)}>
        <Icon name="x" size={14} />
      </button>
    </div>

    {#if capabilities.phase === 'installing'}
      <div class="upd-progress">
        <div
          class="upd-bar" class:indeterminate={pct < 0}
          role="progressbar" aria-valuemin="0" aria-valuemax="100"
          aria-valuenow={pct >= 0 ? pct : undefined} aria-label={title}
        >
          <div class="upd-bar-fill" style={pct >= 0 ? `width:${pct}%` : ''}></div>
        </div>
      </div>
    {/if}

    {#if capabilities.phase === 'error'}
      {#if capabilities.error}<div class="upd-err">{capabilities.error}</div>{/if}
      <!-- Says the true thing rather than the reassuring one: parts that
           finished are on disk, so pressing again continues instead of
           starting the whole 160MB again. -->
      <div class="upd-note">{t('cap.failedSafe')}</div>
      <div class="upd-actions">
        <button class="upd-btn" onclick={() => { hidden = true; dismissCapabilities() }}>
          {t('cap.later')}
        </button>
        <button class="upd-btn upd-btn-go" onclick={retryCapabilities}>{t('cap.retry')}</button>
      </div>
    {/if}
  </div>
{/if}
