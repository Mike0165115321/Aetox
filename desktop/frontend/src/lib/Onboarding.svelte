<script lang="ts">
  // The first screen anyone ever sees, and the only page in the app with no
  // returning users.
  //
  // It was a 560px dialog with two OS dropdowns in it, one of them seventeen
  // providers deep and sorted alphabetically. Then it was a two-column page,
  // which put every choice on screen at once and left half a 1900px window
  // empty. This is the third shape and the first one that is composed rather
  // than laid out: everything on the optical centre, one decision per screen,
  // and every lesser path demoted to a small link under the main one.
  //
  // The rule the layout follows: a screen asks one question. Twelve provider
  // cards is not a question, it is a table — so the keyed providers live behind
  // the button that says you have a key, and open only for someone who does.
  import { onMount } from 'svelte'
  import { t, setLocale, localeNames, i18n, type Locale } from './i18n.svelte'
  import { theme, applyTheme, THEMES, type ThemeName } from './theme.svelte'
  import { readThemeSwatches, type Swatch } from './themeSwatches'
  import Logo from './Logo.svelte'
  import Icon from './Icon.svelte'
  import ProviderMark from './ProviderMark.svelte'
  import {
    SupportedProviders, RequiresAPIKey, AcceptsAPIKey, HasAPIKey,
    ProviderAPIKeyURL, SignInMethods, StartSignIn, CancelSignIn,
  } from '../../wailsjs/go/main/App'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  import { cockpit, switchProvider, submitAPIKey, switchApprovalMode, completeSignIn } from './stores/cockpit.svelte'
  import { DONE_KEY, takeFirstRunReplay } from './firstRun'

  // 0 language · 1 connect · 2 look · 3 approval · 4 done. Connecting comes
  // before the theme because it is the only step that decides whether the app
  // can do anything; picking colours first delays the point by a screen.
  let step = $state(0)
  let visible = $state(false)
  let errorMsg = $state('')

  // acceptsKey is not the negation of requiresKey. Codex requires credentials
  // and has no key to give: it is a ChatGPT subscription reached at
  // chatgpt.com, while any key a user could paste belongs to api.openai.com —
  // a different host, a different bill, and a guaranteed 401.
  type ProviderRow = { name: string; requiresKey: boolean; acceptsKey: boolean; hasKey: boolean }
  let providers = $state<ProviderRow[]>([])
  let signInNames = $state<Set<string>>(new Set())
  let swatches = $state<Record<string, Swatch>>({})
  let allThemes = $state(false)

  // The connect step is two screens: a question, then the providers that
  // answer it. Empty means the question has not been answered yet.
  //
  // Asking first is what lets the wall be short. A first-time user knows
  // whether they pay for ChatGPT every month; they do not know which of
  // seventeen names on an alphabetical list that corresponds to, and a screen
  // that opens with all seventeen makes them find out before they can move.
  type Intent = '' | 'account' | 'key' | 'local'
  let intent = $state<Intent>('')
  let picked = $state('')
  let keyDraft = $state('')
  let keyURL = $state('')

  // busy carries the name of what is being set up, so the button can say it.
  let busy = $state('')
  let settled = $state('')
  let signInPrompt = $state<{ provider: string; kind: string; url: string; user_code?: string } | null>(null)
  let signInCode = $state('')
  let approval = $state('unsafe-only')

  onMount(async () => {
    // Armed by "see the first-run screen" in Settings. It wins over both
    // shortcuts below: on the machine of anyone who would press it, a working
    // provider is always configured, so the wizard would mark itself done and
    // the button would look broken.
    const replay = takeFirstRunReplay()
    if (!replay) {
      if (localStorage.getItem(DONE_KEY)) return
      // An install that already has a working provider key was set up before
      // this wizard existed — never bother it.
      try {
        if (cockpit.model.provider && (await HasAPIKey(cockpit.model.provider))) {
          localStorage.setItem(DONE_KEY, '1')
          return
        }
      } catch {
        /* engine not ready — fall through and show the wizard */
      }
    }
    swatches = readThemeSwatches()
    await loadProviders()
    visible = true
  })

  async function loadProviders() {
    const names = await SupportedProviders()
    providers = await Promise.all(names.map(async (name) => ({
      name,
      requiresKey: await RequiresAPIKey(name),
      acceptsKey: await AcceptsAPIKey(name),
      hasKey: await HasAPIKey(name),
    })))
    signInNames = new Set(((await SignInMethods()) ?? []).map((m: { provider: string }) => m.provider))
  }

  // The second half of the sign-in test is not a fallback for the first: a
  // provider that requires credentials and takes no key can only be signed
  // into, whatever the methods list says. Without it, a failed methods call
  // would drop Codex out of every group and off the screen entirely.
  const isSignIn = (p: ProviderRow) => signInNames.has(p.name) || (p.requiresKey && !p.acceptsKey)
  const signInRows = $derived(providers.filter(isSignIn))
  const keyRows = $derived(providers.filter((p) => !isSignIn(p) && p.requiresKey && p.acceptsKey))
  const localRows = $derived(providers.filter((p) => !p.requiresKey && p.name !== 'aetox'))
  const shownThemes = $derived(allThemes ? THEMES : THEMES.slice(0, 8))

  // The three answers, in the order of how little setup each one costs someone
  // who already has it. An answer with nothing behind it on this machine is not
  // drawn at all — an empty category is not a choice, it is a dead end wearing
  // the clothes of one.
  const INTENTS = [
    {
      key: 'account', icon: 'userRound',
      title: 'onboard.intentAccount', desc: 'onboard.intentAccountDesc',
      wallTitle: 'onboard.wallAccount', wallDesc: 'onboard.wallAccountDesc',
    },
    {
      key: 'key', icon: 'shield',
      title: 'onboard.intentKey', desc: 'onboard.intentKeyDesc',
      wallTitle: 'onboard.wallKey', wallDesc: 'onboard.wallKeyDesc',
    },
    {
      key: 'local', icon: 'monitor',
      title: 'onboard.intentLocal', desc: 'onboard.intentLocalDesc',
      wallTitle: 'onboard.wallLocal', wallDesc: 'onboard.wallLocalDesc',
    },
  ] as const

  function rowsFor(key: Intent): ProviderRow[] {
    if (key === 'account') return signInRows
    if (key === 'key') return keyRows
    if (key === 'local') return localRows
    return []
  }
  const intentDef = $derived(INTENTS.find((i) => i.key === intent) ?? INTENTS[0])

  // What a tile does depends on which question it is answering, and only the
  // keyed one needs a second step. Signing in and a local runtime both have
  // everything they need the moment the tile is pressed.
  function onCell(p: ProviderRow) {
    if (intent === 'key') { openKeyFor(p.name); return }
    picked = p.name
    if (intent === 'account' && !p.hasKey) { startSignIn(p.name); return }
    connect(p.name)
  }

  function finish() {
    localStorage.setItem(DONE_KEY, '1')
    visible = false
  }

  function chooseLocale(code: Locale) {
    setLocale(code)
    step = 1
  }

  async function openKeyFor(name: string) {
    picked = name
    keyDraft = ''
    keyURL = ''
    errorMsg = ''
    try {
      keyURL = await ProviderAPIKeyURL(name)
    } catch {
      /* no page to link to — the field still works */
    }
  }

  // One path for every way in, so the reassurance is identical whichever door
  // was used: the control says what is being set up while it happens, then says
  // it is done, and the screen moves itself on. A first run should never end
  // with a button press that looks like nothing happened.
  async function connect(name: string, key = '') {
    busy = name
    errorMsg = ''
    try {
      if (key.trim()) await submitAPIKey(name, key.trim())
      await switchProvider(name)
      settled = name
      setTimeout(() => { settled = ''; step = 2 }, 850)
    } catch (err) {
      errorMsg = String(err)
    } finally {
      busy = ''
    }
  }

  async function startSignIn(name: string) {
    errorMsg = ''
    signInCode = ''
    busy = name
    try {
      signInPrompt = await StartSignIn(name)
      if (signInPrompt.url) BrowserOpenURL(signInPrompt.url)
      if (signInPrompt.kind !== 'paste') await finishSignIn()
    } catch (err) {
      errorMsg = String(err)
      signInPrompt = null
      busy = ''
    }
  }

  async function finishSignIn() {
    const prompt = signInPrompt
    if (!prompt) return
    busy = prompt.provider
    try {
      await completeSignIn(prompt.provider, signInCode.trim())
      signInPrompt = null
      signInCode = ''
      await loadProviders()
      await connect(prompt.provider)
    } catch (err) {
      errorMsg = String(err)
      busy = ''
    }
  }

  async function abortSignIn() {
    const prompt = signInPrompt
    signInPrompt = null
    signInCode = ''
    busy = ''
    if (prompt) await CancelSignIn(prompt.provider)
  }

  async function finishWithApproval() {
    busy = 'approval'
    errorMsg = ''
    try {
      await switchApprovalMode(approval)
    } catch (err) {
      // Approval is changeable from the composer at any time; a failure here is
      // not worth trapping anyone on the last screen of a setup.
      errorMsg = String(err)
    } finally {
      busy = ''
      step = 4
      setTimeout(() => { if (step === 4) finish() }, 1600)
    }
  }

  const APPROVALS = [
    { value: 'ask', icon: 'hand', label: 'chat.approvalAsk', desc: 'onboard.approvalAskDesc' },
    { value: 'unsafe-only', icon: 'shield', label: 'chat.approvalUnsafeOnly', desc: 'onboard.approvalUnsafeDesc' },
    { value: 'full-access', icon: 'zap', label: 'chat.approvalFullAccess', desc: 'onboard.approvalFullDesc' },
  ] as const
</script>

{#if visible}
  <div class="onboard">
    <!-- Aetox's own ground, the same one an empty chat stands on. -->
    <div class="ob-watermark"><Logo size={520} animate={false} /></div>

    <!-- keyed on step so every screen animates in rather than swapping -->
    {#key step}
      <div class="ob-screen">
        {#if step === 0}
          <div class="ob-logo"><Logo size={54} /></div>
          <h2>{t('onboard.welcomeTitle')}</h2>
          <p class="ob-sub">{t('onboard.welcomeDesc')}</p>
          <div class="ob-stack">
            {#each Object.entries(localeNames) as [code, name]}
              <button class="ob-big" onclick={() => chooseLocale(code as Locale)}>
                <span class="ob-bigt">{name}</span>
                {#if i18n.locale === code}<span class="ob-check"><Icon name="check" size={14} /></span>{/if}
              </button>
            {/each}
          </div>

        {:else if step === 1 && !intent}
          <!-- One question, in the words of what the user already owns rather
               than in brand names. Someone opening this for the first time
               knows whether they pay ChatGPT every month; they do not know
               which of seventeen names on a list that corresponds to. The
               marks on the right are the answer to "like what?" without
               spending a line of text on it. -->
          <h2>{t('onboard.intentTitle')}</h2>
          <p class="ob-sub">{t('onboard.intentDesc')}</p>
          {#if errorMsg}<p class="ob-error">{errorMsg}</p>{/if}
          <div class="ob-stack">
            {#each INTENTS as opt (opt.key)}
              {@const rows = rowsFor(opt.key)}
              {#if rows.length > 0}
                <button class="ob-big tall" onclick={() => (intent = opt.key)}>
                  <span class="ob-bigic"><Icon name={opt.icon} size={16} /></span>
                  <span class="ob-bigt">
                    <span class="t">{t(opt.title)}</span>
                    <span class="d">{t(opt.desc)}</span>
                  </span>
                  <span class="ob-marks">
                    {#each rows.slice(0, 4) as p (p.name)}
                      <span class="mk" title={p.name}><ProviderMark name={p.name} size={15} /></span>
                    {/each}
                    {#if rows.length > 4}<span class="mk more">+{rows.length - 4}</span>{/if}
                  </span>
                </button>
              {/if}
            {/each}
          </div>
          <div class="ob-links"><button class="ob-link" onclick={() => (step = 0)}>{t('onboard.back')}</button></div>

        {:else if step === 1}
          <h2>{t(intentDef.wallTitle)}</h2>
          <p class="ob-sub">{t(intentDef.wallDesc)}</p>

          <div class="ob-wall">
            {#each rowsFor(intent) as p (p.name)}
              <button
                class="ob-cell" class:on={picked === p.name} class:done={settled === p.name}
                disabled={!!busy}
                onclick={() => onCell(p)}
              >
                {#if busy === p.name}<span class="ob-spin"></span>
                {:else}<ProviderMark name={p.name} size={20} />{/if}
                <span class="nm">{p.name}</span>
                {#if p.hasKey}<span class="rdy" title={t('onboard.ready')}></span>{/if}
              </button>
            {/each}
          </div>

          {#if intent === 'key' && picked}
            <div class="ob-drawer">
              <div class="ob-keyrow">
                <input
                  class="ob-input" type="password" placeholder={t('onboard.keyPlaceholderFor', { name: picked })}
                  bind:value={keyDraft}
                  onkeydown={(e) => e.key === 'Enter' && connect(picked, keyDraft)}
                />
                <button class="ob-go" disabled={!!busy} onclick={() => connect(picked, keyDraft)}>
                  {#if busy === picked}<span class="ob-spin"></span>
                  {:else if settled === picked}<Icon name="check" size={14} />
                  {:else}{t('onboard.connect')}{/if}
                </button>
              </div>
              {#if keyURL}
                <button class="ob-link" onclick={() => BrowserOpenURL(keyURL)}>
                  {t('onboard.getKey')} <Icon name="externalLink" size={11} />
                </button>
              {/if}
            </div>
          {/if}

          {#if busy || settled}
            <p class="ob-fine">
              {#if settled}{t('onboard.connected', { name: settled })}
              {:else if signInPrompt}{t('onboard.signInWaiting')}
              {:else}{t('onboard.connecting', { name: busy })}{/if}
            </p>
          {/if}
          {#if signInPrompt?.user_code}
            <p class="ob-fine">{t('onboard.signInCode', { code: signInPrompt.user_code })}</p>
          {/if}
          {#if intent === 'local'}<p class="ob-fine">{t('onboard.localFine')}</p>{/if}
          {#if errorMsg}<p class="ob-error">{errorMsg}</p>{/if}

          <div class="ob-links">
            {#if signInPrompt}
              <button class="ob-link" onclick={abortSignIn}>{t('settings.signInCancel')}</button>
            {:else}
              <button class="ob-link" disabled={!!busy} onclick={() => { intent = ''; picked = ''; errorMsg = '' }}>{t('onboard.back')}</button>
            {/if}
          </div>

        {:else if step === 2}
          <h2>{t('onboard.themeTitle')}</h2>
          <p class="ob-sub">{t('onboard.themeDesc')}</p>
          <div class="ob-swatches">
            {#each shownThemes as th (th.value)}
              <button
                class="ob-swatch" class:on={theme.name === th.value} title={th.label}
                style:background={swatches[th.value]?.bg}
                onclick={() => applyTheme(th.value as ThemeName)}
              >
                <span class="accent" style:background={swatches[th.value]?.accent}></span>
              </button>
            {/each}
          </div>
          <button class="ob-link" onclick={() => (allThemes = !allThemes)}>
            {allThemes ? t('onboard.themeFewer') : t('onboard.themeAll', { n: String(THEMES.length) })}
          </button>
          <div class="ob-stack tight">
            <button class="ob-big primary" onclick={() => (step = 3)}>
              <span class="ob-bigt">{t('onboard.next')}</span>
            </button>
          </div>
          <!-- Back, on every screen that has one behind it. Not a skip: it
               returns a decision to the user rather than removing it. -->
          <div class="ob-links"><button class="ob-link" onclick={() => { step = 1; intent = '' }}>{t('onboard.back')}</button></div>

        {:else if step === 3}
          <h2>{t('onboard.approvalTitle')}</h2>
          <p class="ob-sub">{t('onboard.approvalDesc')}</p>
          <div class="ob-stack">
            {#each APPROVALS as opt (opt.value)}
              <button class="ob-row" class:on={approval === opt.value} onclick={() => (approval = opt.value)}>
                <span class="ic"><Icon name={opt.icon} size={17} /></span>
                <span class="txt">
                  <span class="t">{t(opt.label)}</span>
                  <span class="d">{t(opt.desc)}</span>
                </span>
                {#if opt.value === 'unsafe-only'}<span class="ob-tag">{t('onboard.recommended')}</span>{/if}
              </button>
            {/each}
          </div>
          {#if errorMsg}<p class="ob-error">{errorMsg}</p>{/if}
          <div class="ob-stack tight">
            <button class="ob-big primary" disabled={!!busy} onclick={finishWithApproval}>
              {#if busy}<span class="ob-spin"></span>{/if}
              <span class="ob-bigt">{t('onboard.start')}</span>
            </button>
          </div>
          <div class="ob-links"><button class="ob-link" disabled={!!busy} onclick={() => (step = 2)}>{t('onboard.back')}</button></div>

        {:else}
          <div class="ob-done"><Icon name="check" size={24} /></div>
          <h2>{t('onboard.readyTitle')}</h2>
          <p class="ob-sub">{t('onboard.readyDesc')}</p>
        {/if}
      </div>
    {/key}

    <!-- Where you are, without a number: four steps is few enough to draw. -->
    <div class="ob-dots">
      {#each [0, 1, 2, 3] as i}<i class:on={step === i} class:past={step > i}></i>{/each}
    </div>
  </div>
{/if}
