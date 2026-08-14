<script lang="ts">
  // The two things the update feature was missing from the user's side: being
  // told, and being shown. internal/update could already download, verify and
  // swap the whole app — but the only way to reach it was to go looking in
  // Settings, and the only sign it was working was a word on a button.
  //
  // So, two surfaces, one at a time:
  //
  //   - the notice, bottom-right, when the automatic check found something. It
  //     is an offer, not an alert: it never covers the composer, "later" is a
  //     real answer, and nothing has been downloaded at the moment it appears.
  //   - the dialog, once the user says yes. Modal on purpose — the exe under
  //     this window is being replaced and the app is about to restart itself,
  //     so there is nothing useful to do behind it, and a progress bar that can
  //     be lost behind the window is a progress bar nobody watches.
  //
  // A failure closes neither: it leaves the old build installed and untouched,
  // says so, and re-arms — which is exactly why the dialog stays up to say it.
  import { t } from './i18n.svelte'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  import { updater, updateOffered, dismissUpdate, startUpdate } from './selfUpdate.svelte'

  // The dialog owns the screen from the click until the window closes. Failure
  // keeps it up: the sentence explaining what went wrong is the whole reason it
  // is still there.
  const busy = $derived(updater.applying || updater.restarting)
  const showDialog = $derived(busy || updater.error !== '')

  const label = $derived(
    updater.restarting
      ? t('update.restarting')
      : updater.pct >= 0
        ? t('update.workingPct', { pct: String(updater.pct) })
        : t('update.working'),
  )
</script>

{#if updateOffered() && !showDialog}
  <div class="upd-notice" role="status">
    <div class="upd-notice-title">{t('update.ready', { version: updater.status?.latest ?? '' })}</div>
    <!-- Three endings, one action each — the same three internal/update
         already decided between (Status.canAuto / .hint). Scoop installed us,
         so Scoop upgrades us: Aetox does not write into someone else's
         package directory. -->
    <div class="upd-notice-body">
      {updater.status?.canAuto
        ? t('update.readyAuto')
        : updater.status?.hint
          ? t('update.readyCommand')
          : t('update.readyManual')}
    </div>
    {#if !updater.status?.canAuto && updater.status?.hint}
      <code class="upd-cmd">{updater.status.hint}</code>
    {/if}
    <div class="upd-notice-actions">
      <button class="upd-btn" onclick={dismissUpdate}>{t('update.later')}</button>
      {#if updater.status?.canAuto}
        <button class="upd-btn upd-btn-go" onclick={startUpdate}>{t('update.now')}</button>
      {:else}
        <button class="upd-btn upd-btn-go" onclick={() => BrowserOpenURL(updater.status?.url ?? '')}>
          {t('update.openRelease')}
        </button>
      {/if}
    </div>
  </div>
{/if}

{#if showDialog}
  <div class="upd-overlay" role="dialog" aria-modal="true" aria-labelledby="upd-dialog-label">
    <div class="upd-card">
      <div class="upd-card-title">Aetox</div>
      {#if updater.error}
        <div id="upd-dialog-label" class="upd-card-msg">{t('update.failed')}</div>
        <div class="upd-card-err">{updater.error}</div>
        <div class="upd-card-note">{t('update.failedSafe')}</div>
        <div class="upd-notice-actions">
          <button class="upd-btn" onclick={() => BrowserOpenURL(updater.status?.url ?? '')}>
            {t('update.openRelease')}
          </button>
          <button class="upd-btn upd-btn-go" onclick={startUpdate}>{t('update.retry')}</button>
        </div>
      {:else}
        <!-- aria-live, so a screen reader hears the percentage move without
             the user having to go hunting for the bar. -->
        <div id="upd-dialog-label" class="upd-card-msg" aria-live="polite">{label}</div>
        <div
          class="upd-bar" class:indeterminate={updater.pct < 0}
          role="progressbar"
          aria-valuemin="0" aria-valuemax="100"
          aria-valuenow={updater.pct >= 0 ? updater.pct : undefined}
          aria-label={label}
        >
          <div class="upd-bar-fill" style={updater.pct >= 0 ? `width:${updater.pct}%` : ''}></div>
        </div>
        <!-- Said once, up front: the app closes itself and comes back. Without
             it, a window that vanishes mid-progress reads as a crash. -->
        <div class="upd-card-note">{t('update.willRestart')}</div>
      {/if}
    </div>
  </div>
{/if}
