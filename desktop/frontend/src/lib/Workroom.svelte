<script lang="ts">
  // ห้องทำงาน (COMPANY.md §2, DECISIONS §158): the only room behind Aetox ทีม.
  //
  // A ห้องทำงาน is a run written down — the steps a job goes through, and which
  // เอเจน sits at each one. None of that is built yet, and this page says so in
  // those words rather than drawing an empty list that implies the list works.
  //
  // It exists at all because the door needs somewhere to land: a `soon` button
  // is a name among rooms that open, and this is the only room there is. What
  // fills in here is the roster of saved ห้องทำงาน and the run board (§158.7);
  // the header, the shell and the empty state are already the ones they will
  // use, so building it is filling this in rather than replacing it.
  //
  // NO BACK BUTTON, and that is the one way this page differs from every other
  // one on the same surface. The others are drawn over a conversation you came
  // from and go back to. This one is the ทีม door's home — there is nothing
  // behind it — so it renders inside the layout (App.svelte's <main>) with the
  // door menu and the rooms row still on screen. A "กลับไปที่แอป" button here
  // would have to invent a destination, and the one it used to invent was
  // another door's chat.
  import { t } from './i18n.svelte'
  import Icon from './Icon.svelte'
</script>

<div class="page-shell in-main">
  <header class="page-head">
    <div class="page-title">
      <h2>{t('desk.lines')}</h2>
      <p>{t('lines.intro')}</p>
    </div>
  </header>

  <div class="page-body">
    <div class="settings-inner">
      <!-- Symmetric emptiness: one card, centred copy, no dashed placeholder
           slots pretending to be cards that will exist. The line that matters
           is the second one, because it is the only thing a person standing
           here can act on. -->
      <div class="settings-card room-empty">
        <div class="room-empty-mark"><Icon name="layoutList" size={22} /></div>
        <div class="room-empty-title">{t('lines.emptyTitle')}</div>
        <p class="room-empty-body">{t('lines.emptyBody')}</p>
      </div>
    </div>
  </div>
</div>

<style>
  /* Local because nothing else on this door has a shape yet. When the roster
     and the run board land, whatever they share moves to style.css with them. */
  .room-empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
    gap: var(--sp-3, 12px);
    padding: var(--sp-8, 48px) var(--sp-6, 32px);
  }
  .room-empty-mark {
    display: grid;
    place-items: center;
    width: 44px;
    height: 44px;
    border-radius: 12px;
    background: var(--surface-2, rgba(127, 127, 127, 0.1));
    color: var(--text-dim, inherit);
  }
  .room-empty-title {
    font-weight: 600;
  }
  .room-empty-body {
    margin: 0;
    max-width: 46ch;
    color: var(--text-dim, inherit);
    line-height: 1.6;
  }
</style>
