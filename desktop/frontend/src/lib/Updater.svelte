<script lang="ts">
  // The two things the update feature was missing from the user's side: being
  // told, and being shown. internal/update could already download, verify and
  // swap the whole app — but the only way to reach it was to go looking in
  // Settings, and the only sign it was working was a word on a button.
  //
  // One card, four things to say (§107):
  //
  //   offer       — a newer release exists. Nothing downloaded yet.
  //   downloading — bytes moving, in MB, because that is the unit the number
  //                 next to a progress bar is actually in.
  //   ready       — it is on disk. Restart now, or don't: closing the app
  //                 tonight installs it just the same.
  //   error       — what failed, and that the running build is untouched.
  //
  // Deliberately a card and not a modal. Nothing here interrupts the app: the
  // download runs while the user keeps working, and the restart is theirs to
  // time. A scrim over the window would be claiming otherwise. The × dismisses
  // it without stopping anything, and it comes back the moment the phase
  // changes — a download that finished or failed behind a closed card is news
  // the user still has to get.
  import { t } from './i18n.svelte'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  import Logo from './Logo.svelte'
  import Icon from './Icon.svelte'
  import {
    updater, updateOffered, updatePct, dismissUpdate, startDownload, restartToUpdate,
  } from './selfUpdate.svelte'

  const busy = $derived(updater.phase !== 'idle')

  // Everything a release page would tell you in one line, minus the release
  // page. The date matters more than it looks: "v0.9.7" alone says nothing
  // about whether this is an hour old or a month old.
  const version = $derived(updater.staged || updater.status?.latest || '')

  // The × hides what the card is currently saying — not the card forever. Any
  // change to what it WOULD say (a finished download, a failure, a newer
  // release than the one waved off) is news the user has not seen yet, so the
  // card comes back. Keyed on phase AND version, because a second release
  // arriving while still in the offer phase is a phase change of exactly zero.
  let hidden = $state(false)
  let said = $state('')
  $effect(() => {
    const now = `${updater.phase}:${version}`
    if (now !== said) {
      said = now
      hidden = false
    }
  })

  const show = $derived(!hidden && (busy || updateOffered()))
  const published = $derived(
    updater.status?.publishedAt
      ? new Date(updater.status.publishedAt).toLocaleDateString(undefined, {
          year: 'numeric', month: 'long', day: 'numeric',
        })
      : '',
  )

  const MB = 1024 * 1024
  const mb = (bytes: number) => (bytes / MB).toFixed(1)
  const pct = $derived(updatePct())

  const title = $derived(
    updater.phase === 'downloading'
      ? t('update.downloading', { version })
      : updater.phase === 'ready'
        ? t('update.readyToRestart', { version })
        : updater.phase === 'restarting'
          ? t('update.restarting')
          : updater.phase === 'error'
            ? t('update.failed')
            : t('update.ready', { version }),
  )
</script>

{#if show}
  <div class="upd-card" role="status" aria-live="polite">
    <div class="upd-head">
      <div class="upd-icon"><Logo size={26} animate={false} /></div>
      <div class="upd-headings">
        <div class="upd-title">{title}</div>
        {#if published}
          <div class="upd-date">{published}</div>
        {/if}
      </div>
      <button class="upd-x" aria-label={t('update.hide')} onclick={() => (hidden = true)}>
        <Icon name="x" size={14} />
      </button>
    </div>

    {#if updater.phase === 'downloading' || updater.phase === 'restarting'}
      <div class="upd-progress">
        <div class="upd-progress-row">
          <span>{t('update.progress')}</span>
          <!-- Bytes, not a percentage, when the size is known: "53.2MB / 53.4MB"
               says how much is left in the unit the user's connection is in. -->
          {#if updater.total > 0}
            <span class="upd-bytes">{mb(updater.done)}MB / {mb(updater.total)}MB</span>
          {/if}
        </div>
        <div
          class="upd-bar" class:indeterminate={pct < 0}
          role="progressbar" aria-valuemin="0" aria-valuemax="100"
          aria-valuenow={pct >= 0 ? pct : undefined} aria-label={title}
        >
          <div class="upd-bar-fill" style={pct >= 0 ? `width:${pct}%` : ''}></div>
        </div>
      </div>
    {/if}

    {#if updater.error}
      <div class="upd-err">{updater.error}</div>
      <div class="upd-note">{t('update.failedSafe')}</div>
    {/if}

    {#if updater.phase === 'idle'}
      <!-- Three endings, one action each — the same three internal/update
           already decided between (Status.canAuto / .hint). Scoop installed us,
           so Scoop upgrades us: Aetox does not write into someone else's
           package directory. -->
      <div class="upd-note">
        {updater.status?.canAuto
          ? t('update.offerAuto')
          : updater.status?.hint
            ? t('update.offerCommand')
            : t('update.offerManual')}
      </div>
      {#if !updater.status?.canAuto && updater.status?.hint}
        <code class="upd-cmd">{updater.status.hint}</code>
      {/if}
    {/if}

    {#if updater.phase === 'ready'}
      <div class="upd-note">{t('update.readyNote')}</div>
    {/if}

    {#if updater.phase !== 'downloading' && updater.phase !== 'restarting'}
      <div class="upd-actions">
        <button class="upd-btn" onclick={() => { hidden = true; dismissUpdate() }}>
          {t('update.later')}
        </button>
        {#if updater.phase === 'ready' || (updater.phase === 'error' && updater.staged)}
          <button class="upd-btn upd-btn-go" onclick={restartToUpdate}>{t('update.restartNow')}</button>
        {:else if updater.phase === 'error'}
          <button class="upd-btn upd-btn-go" onclick={startDownload}>{t('update.retry')}</button>
        {:else if updater.status?.canAuto}
          <button class="upd-btn upd-btn-go" onclick={startDownload}>{t('update.now')}</button>
        {:else}
          <button class="upd-btn upd-btn-go" onclick={() => BrowserOpenURL(updater.status?.url ?? '')}>
            {t('update.openRelease')}
          </button>
        {/if}
      </div>
    {/if}
  </div>
{/if}
