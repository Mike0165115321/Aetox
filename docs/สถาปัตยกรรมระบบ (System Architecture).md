┌─────────────────┐
│  👤 ผู้ใช้      │
│  (Discord/CLI)  │
└────────┬────────┘
         │ Command (ภาษาไทย)
         ▼
┌─────────────────┐
│  🧠 Intent      │
│  Extractor      │
│  • LLM: Qwen3-8B│
│  • Output: JSON │
└────────┬────────┘
         │ {tool, action, params, confidence}
         ▼
┌─────────────────┐
│  🎯 Dispatcher  │◄───────────────┐
│  • Async Orchestrator           │
│  • Retry Logic + Timeout        │
│  • Critic Feedback Loop         │
└────────┬────────┘               │
         │                         │
    ┌────┴─────┐                   │
    ▼          ▼                   │
┌────────┐ ┌────────┐             │
│🔧 Tool │ │💬 Chat │             │
│Registry│ │Stream  │             │
└────┬───┘ └────────┘             │
     │                             │
     ▼                             │
┌─────────────────┐               │
│  💾 Working    │               │
│  Memory         │               │
│  • RAM Cache   │               │
│  • Disk Backup │               │
│  • Context Mgr │               │
└────────┬────────┘               │
         │                        │
         ▼                        │
┌─────────────────┐               │
│  🔍 Critic     │───────────────┘
│  • Quality Check              │
│  • Auto-Retry Trigger         │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  📤 Response   │
│  • Formatted  │
│  • Saved      │
└────────────────┘


