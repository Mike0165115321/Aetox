<!--
  PARKED 2026-08-14 — nothing renders this. The owner shelved the phone
  surface after the working slice made the real scope visible: a phone that can
  drive this machine needs its own design for display, navigation and security,
  and it will be built as its own client rather than as this panel's other half.

  Kept rather than deleted because it is the desktop half of a pairing flow that
  works and is tested, and because deleting it would throw away the one part of
  the decision that was settled: what this panel does NOT have on it.

  To bring it back, in Sidebar.svelte: import this component, add
  `let remoteOpen = $state(false)`, a menu button setting it true, and mount
  `{#if remoteOpen}<MobileRemote onClose={...} />{/if}` after </aside>. The
  strings (`sidebar.mobileRemote`, `remote.*`) are already in both locales.

  Read docs/architecture/mobile-remote-2026-08-14.md before changing anything
  here — especially the security section, which is why this is parked and not
  shipped.

  The desktop half of pairing a phone (desktop/remote.go).

  This panel is the whole setup experience, and its shape is the point: there
  is nothing on it to fill in. No port, no address, no password — the owner's
  rule for this feature was "ไม่เอาแบบมาตั้งค่าซ้ำซ้อน", and a field here would
  be the first thing to break it. Opening the panel starts the listener and
  mints a QR; closing it leaves both alone.
-->
<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import {
    StartMobileRemote, StopMobileRemote, MobileRemoteStatus, MobileRemoteQR, RevokeDevice,
  } from '../../wailsjs/go/main/App'
  import { t } from './i18n.svelte'
  import Icon from './Icon.svelte'

  let { onClose }: { onClose: () => void } = $props()

  type Device = { id: string; label: string; pairedAt: string; lastSeen: string }
  type Status = {
    running: boolean; address: string; subnet: string
    devices: Device[]; error: string
  }

  let status = $state<Status>({ running: false, address: '', subnet: '', devices: [], error: '' })
  let qr = $state('')
  let busy = $state(true)
  let poll: ReturnType<typeof setInterval> | undefined

  // The QR is refreshed alongside the device list because the token behind it
  // expires — a panel left open must never show a code that stopped working.
  async function refresh() {
    status = await MobileRemoteStatus()
    if (status.running) qr = await MobileRemoteQR()
  }

  onMount(async () => {
    try {
      status = await StartMobileRemote()
      if (status.running) qr = await MobileRemoteQR()
      poll = setInterval(refresh, 5000)
    } finally {
      busy = false
    }
  })

  onDestroy(() => clearInterval(poll))

  // Turning the door off is not un-pairing: a phone paired last week is still
  // paired tomorrow, so this only closes the listener.
  async function stop() {
    busy = true
    try {
      status = await StopMobileRemote()
      qr = ''
    } finally {
      busy = false
    }
  }

  async function start() {
    busy = true
    try {
      status = await StartMobileRemote()
      if (status.running) qr = await MobileRemoteQR()
    } finally {
      busy = false
    }
  }

  async function revoke(id: string) {
    await RevokeDevice(id)
    await refresh()
  }
</script>

<div class="scrim" role="presentation" onclick={onClose}></div>

<div class="panel" role="dialog" aria-modal="true" aria-label={t('remote.title')}>
  <header>
    <span class="ic"><Icon name="smartphone" size={16} /></span>
    <h2>{t('remote.title')}</h2>
    <button class="x" onclick={onClose} aria-label={t('settings.close')}>
      <Icon name="x" size={15} />
    </button>
  </header>

  {#if status.error}
    <p class="err">{status.error}</p>
  {/if}

  {#if status.running && qr}
    <p class="lead">{t('remote.scanLead')}</p>
    <img class="qr" src={qr} alt="QR" />
    <p class="where">
      <span class="dot"></span>
      {t('remote.listeningOn')} <code>{status.address}</code>
    </p>
    <p class="note">{t('remote.lanOnly')}</p>
  {:else if busy}
    <p class="lead">{t('remote.starting')}</p>
  {:else}
    <p class="lead">{t('remote.off')}</p>
    <button class="wide" onclick={start}>{t('remote.turnOn')}</button>
  {/if}

  {#if status.devices.length}
    <h3>{t('remote.paired')}</h3>
    {#each status.devices as d (d.id)}
      <div class="dev">
        <span class="nm">{d.label}</span>
        <span class="ts">{d.pairedAt.slice(0, 10)}</span>
        <button class="revoke" onclick={() => revoke(d.id)}>{t('remote.revoke')}</button>
      </div>
    {/each}
  {/if}

  {#if status.running}
    <button class="wide off" onclick={stop} disabled={busy}>{t('remote.turnOff')}</button>
  {/if}
</div>

<style>
  .scrim {
    position: fixed; inset: 0; background: rgba(4, 7, 12, .6);
    backdrop-filter: blur(2px); z-index: 60;
  }
  .panel {
    position: fixed; z-index: 61; top: 50%; left: 50%; transform: translate(-50%, -50%);
    width: min(380px, calc(100vw - 32px)); max-height: calc(100vh - 48px); overflow: auto;
    background: var(--panel, #131926); border: 1px solid var(--line, #1f2836);
    border-radius: 16px; padding: 18px; text-align: center;
    box-shadow: 0 24px 60px rgba(0, 0, 0, .5);
  }
  header { display: flex; align-items: center; gap: 8px; text-align: left; margin-bottom: 14px }
  header h2 { font-size: 14px; font-weight: 600; margin: 0; flex: 1 }
  .ic { display: grid; place-items: center; opacity: .8 }
  .x {
    background: none; border: none; color: inherit; opacity: .6;
    cursor: pointer; padding: 2px; display: grid; place-items: center;
  }
  .x:hover { opacity: 1 }
  .lead { font-size: 13px; color: var(--dim, #8b98ab); margin: 0 0 14px }
  .qr {
    width: 240px; height: 240px; background: #fff;
    padding: 12px; border-radius: 14px; display: block; margin: 0 auto;
  }
  .where { font-size: 12px; color: var(--dim, #8b98ab); margin: 14px 0 0 }
  .where code { color: var(--accent, #6ea8fe); font-size: 12px }
  .dot {
    display: inline-block; width: 7px; height: 7px; border-radius: 50%;
    background: #4ade80; margin-right: 4px; vertical-align: middle;
  }
  .note { font-size: 11.5px; color: var(--dim, #8b98ab); opacity: .8; margin: 8px 0 0; line-height: 1.5 }
  .err {
    font-size: 12.5px; color: #f87171; background: rgba(248, 113, 113, .1);
    border: 1px solid rgba(248, 113, 113, .3); border-radius: 9px;
    padding: 9px; margin: 0 0 12px; text-align: left;
  }
  h3 {
    font-size: 11px; text-transform: uppercase; letter-spacing: .06em;
    color: var(--dim, #8b98ab); text-align: left; margin: 20px 0 8px;
  }
  .dev { display: flex; align-items: center; gap: 8px; font-size: 13px; padding: 6px 0 }
  .dev .nm { flex: 1; text-align: left }
  .dev .ts { color: var(--dim, #8b98ab); font-size: 11.5px }
  .revoke {
    background: none; border: none; color: #f87171; font: inherit;
    font-size: 12px; cursor: pointer; padding: 2px 4px;
  }
  .wide {
    width: 100%; margin-top: 16px; padding: 10px; border-radius: 10px;
    border: 1px solid var(--line, #1f2836); background: #1a2130;
    color: inherit; font: inherit; font-weight: 600; cursor: pointer;
  }
  .wide.off { color: #f87171 }
  .wide:disabled { opacity: .5 }
</style>
