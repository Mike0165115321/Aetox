<script lang="ts">
  import type { ChatMessage, TaskState } from './types'
  import TaskTimeline from './TaskTimeline.svelte'

  let {
    messages, task, governanceFile, onSend,
  }: {
    messages: ChatMessage[]
    task: TaskState
    governanceFile: string
    onSend: (text: string) => void
  } = $props()

  const tabs = ['Chat', 'Aetox.md Map']
  let activeTab = $state('Chat')
  let draft = $state('')

  function submit() {
    if (!draft.trim()) return
    onSend(draft)
    draft = ''
  }
  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      submit()
    }
  }
</script>

<div class="tabs">
  {#each tabs as tab}
    <button class="tab" class:active={activeTab === tab} onclick={() => (activeTab = tab)}>
      {tab}
    </button>
  {/each}
</div>

{#if activeTab === 'Chat'}
  <div class="chat">
    {#each messages as m}
      <div class="msg {m.role === 'user' ? 'user' : 'bot'}">
        <div class="who">{m.role === 'user' ? '🧑' : '🦅'}</div>
        <div>
          <div class="bubble">
            {#if m.role === 'agent'}
              <div class="name"><b>Aetox Agent</b>
                {#if m.tag}<span class="tag think">{m.tag}</span>{/if}
              </div>
            {/if}
            <span class="body-text">{m.text}</span>
            <div class="time">{m.time}</div>
          </div>
        </div>
      </div>
    {/each}

    <TaskTimeline steps={task.steps} elapsed={task.elapsed} />
  </div>

  <div class="composer">
    <div class="box">
      <textarea
        class="input"
        rows="1"
        placeholder="Type your command or request… (ใช้ / เพื่อดูคำสั่ง)"
        bind:value={draft}
        onkeydown={onKeydown}
      ></textarea>
      <div class="tools">
        <span class="icobtn">📎</span>
        <span class="icobtn">🖼</span>
        <span class="icobtn">⌥</span>
        <button class="send" aria-label="Send" onclick={submit}>➤</button>
      </div>
    </div>
  </div>
{:else}
  <div class="map-view">
    <div class="map-card">
      <div class="map-icon">🗺</div>
      <div class="map-title">{governanceFile} Map</div>
      <p class="map-sub">มุมมองโครงสร้าง governance ของ {governanceFile} — จะเชื่อมกับ Go core ที่ parse ไฟล์จริงในขั้นต่อไป</p>
    </div>
  </div>
{/if}
