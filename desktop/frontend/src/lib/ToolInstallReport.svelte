<script lang="ts">
  // What is about to be put on this machine, said before it is.
  //
  // The lock on an agent card tells somebody they cannot use a teammate until a
  // tool is installed. A sentence like that owes an answer to "installed what,
  // exactly, from where, and can I undo it" — and answering it in the moment
  // the question occurs is the difference between a button somebody presses and
  // a button somebody trusts. Owner, 30 ส.ค.: *"ตอนกดติดตั้งให้แสดงรายงานด้วยว่า
  // จะติดตั้งอะไร และบอกว่าปลอดภัย ทิ้งเว็บให้"*.
  //
  // The four assurances are not per-tool copy. They are properties every entry
  // in internal/capability has by construction — the checksum is verified
  // before anything is unpacked, nothing elevates, nothing is written to PATH,
  // and everything lands under one folder — so they are written once, here, and
  // are true of whatever the plan happens to name.
  //
  // Same shell as ConfirmDialog and for its reasons, not to save work: this is
  // the same event to a user, a modal asking for one decision, and a second
  // shape for it would read as a different kind of question.
  import { onMount } from 'svelte'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  import { main } from '../../wailsjs/go/models'
  import { t } from './i18n.svelte'
  import Icon from './Icon.svelte'

  let { plan, title, onCancel, onConfirm }: {
    plan: main.ToolInstallPlan
    title: string
    onCancel: () => void
    onConfirm: () => void
  } = $props()

  let goEl = $state<HTMLButtonElement | null>(null)
  onMount(() => goEl?.focus())

  // Whole megabytes. A download quoted to one decimal place invites somebody to
  // check it against what actually arrives, and the manifest's own figure is
  // approximate by construction (the real number is Content-Length).
  const mb = (bytes: number) => Math.round(bytes / (1 << 20))

  // One licence line and one source list for the whole archive rather than a
  // column of repeats: two parts under the same licence said it twice, and the
  // question a reader has is about what they are being handed, not per file.
  const licences = $derived([...new Set(plan.parts.map((p) => p.license).filter(Boolean))].join(' · '))
  const sources = $derived([...new Set(plan.parts.map((p) => p.homepage).filter(Boolean))] as string[])
  const host = (url: string) => url.replace(/^https?:\/\//, '').replace(/^www\./, '').replace(/\/.*$/, '')

  const ASSURANCES = ['lock.safeSum', 'lock.safeAdmin', 'lock.safePath', 'lock.safeUndo'] as const

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.stopPropagation()
      onCancel()
    }
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="confirm-overlay" role="alertdialog" tabindex="-1" aria-modal="true"
  aria-labelledby="tool-install-title" onkeydown={onKeydown}>
  <button class="confirm-backdrop" aria-label={t('settings.cancel')} onclick={onCancel}></button>
  <div class="confirm-card">
    <h3 id="tool-install-title" class="confirm-title">{title}</h3>
    <p class="confirm-message">{t('lock.reportIntro')}</p>

    <div class="tool-plan">
      <div class="tool-plan-row">
        <span class="k">{t('lock.what')}</span>
        <span class="v">
          {#each plan.parts as part (part.id)}
            <span class="one">
              {part.title || part.id}
              {#if part.includes}<span class="also">{t('lock.includes', { what: part.includes })}</span>{/if}
            </span>
          {/each}
        </span>
      </div>
      {#if plan.dest}
        <div class="tool-plan-row">
          <span class="k">{t('lock.dest')}</span>
          <span class="path">{plan.dest}</span>
        </div>
      {/if}
      <div class="tool-plan-row">
        <span class="k">{t('lock.size')}</span>
        <span class="v strong">{t('lock.mb', { n: mb(plan.totalBytes) })}</span>
        {#if licences}<span class="sz">{licences}</span>{/if}
      </div>
      {#if sources.length > 0}
        <div class="tool-plan-row">
          <span class="k">{t('lock.source')}</span>
          <span class="v links">
            {#each sources as url (url)}
              <button class="linklike" onclick={() => BrowserOpenURL(url)}>
                {host(url)} <Icon name="externalLink" size={11} />
              </button>
            {/each}
          </span>
        </div>
      {/if}
    </div>

    <ul class="tool-safe">
      {#each ASSURANCES as key (key)}
        <li><Icon name="check" size={14} /><span>{t(key)}</span></li>
      {/each}
    </ul>

    <div class="confirm-actions">
      <button class="ctrl" onclick={onCancel}>{t('lock.notNow')}</button>
      <button class="ctrl ctrl-primary" bind:this={goEl} onclick={onConfirm}>
        <Icon name="download" size={14} />
        {t('lock.installMb', { n: mb(plan.totalBytes) })}
      </button>
    </div>
  </div>
</div>
