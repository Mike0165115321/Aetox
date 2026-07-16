// Sample cockpit state matching แผน Ui.png. This is the ONLY place sample content
// lives; components never import it. MockSource serves it; swap in WailsSource to
// feed real data from the Go core without touching a single component.

import type { CockpitState } from './types'

export const mockState: CockpitState = {
  project: {
    name: 'aetox-cli',
    path: 'E:\\Aetox\\Source\\aetox-cli',
    branch: 'dev',
    extraBranches: 3,
    governanceFile: 'Aetox.md',
    governanceLoaded: true,
  },

  model: {
    provider: 'Groq / qwen3-32b',
    thinkLevel: 'Low',
    speed: 'Jow',
    contextPct: 18,
    contextUsed: '28.4K',
    contextMax: '160K',
    approval: 'Auto (Safe)',
  },

  tree: [
    { label: 'aetox-cli', kind: 'dir', depth: 0, open: true, active: true, icon: '📂' },
    { label: 'Aetox.md', kind: 'file', depth: 1, status: 'M', icon: '📄' },
    { label: 'README.md', kind: 'file', depth: 1, status: 'M', icon: '📄' },
    { label: '.aetoxignore', kind: 'file', depth: 1, icon: '📄' },
    { label: 'cmd', kind: 'dir', depth: 1, icon: '📁' },
    { label: 'internal', kind: 'dir', depth: 1, open: true, icon: '📂' },
    { label: 'app', kind: 'dir', depth: 2, icon: '📁' },
    { label: 'audit', kind: 'dir', depth: 2, icon: '📁' },
    { label: 'cognitive', kind: 'dir', depth: 2, icon: '📁' },
    { label: 'config', kind: 'dir', depth: 2, icon: '📁' },
    { label: 'memory', kind: 'dir', depth: 2, icon: '📁' },
    { label: 'model', kind: 'dir', depth: 2, icon: '📁' },
    { label: 'provider', kind: 'dir', depth: 2, icon: '📁' },
    { label: 'safety', kind: 'dir', depth: 2, icon: '📁' },
    { label: 'skill', kind: 'dir', depth: 2, icon: '📁' },
    { label: 'think', kind: 'dir', depth: 2, icon: '📁' },
    { label: 'turn', kind: 'dir', depth: 2, open: true, icon: '📂' },
    { label: 'executor.go', kind: 'file', depth: 3, status: 'M', icon: '🐹' },
    { label: 'infer.go', kind: 'file', depth: 3, status: 'M', active: true, icon: '🐹' },
    { label: 'result.go', kind: 'file', depth: 3, icon: '🐹' },
    { label: 'infer_test.go', kind: 'file', depth: 3, status: 'U', icon: '🐹' },
    { label: 'executor_test.go', kind: 'file', depth: 3, icon: '🐹' },
    { label: 'result_test.go', kind: 'file', depth: 3, icon: '🐹' },
    { label: 'types', kind: 'dir', depth: 2, icon: '📁' },
  ],

  sessions: [
    { title: 'Fix parser bug', ago: '2m ago', active: true },
    { title: 'Refactor executor', ago: '1h ago' },
    { title: 'Context loading issue', ago: '3h ago' },
  ],

  chat: [
    {
      role: 'user',
      text: 'แก้บั๊ก parser ใน internal/turn ให้หน่อย\nตอนนี้ test พัง info: quoted path กับ escape',
      time: '10:15 AM',
    },
    {
      role: 'agent',
      tag: 'Thinking (low)',
      text: 'เข้าใจแล้วครับ เดี๋ยวผมตรวจสอบ parser ใน internal/turn และแก้บั๊กเกี่ยวกับ quoted path และ escape ให้ครับ',
      time: '10:15 AM',
    },
  ],

  task: {
    elapsed: '2m 34s',
    steps: [
      { time: '10:15:21', title: 'Intent Recognition', detail: 'coding.fix · แก้บั๊กใน parser', status: 'done' },
      { time: '10:15:22', title: 'Load Context', detail: 'Aetox.md, .aetoxignore', status: 'done' },
      { time: '10:15:23', title: 'Read Files', detail: 'internal/turn/infer.go, infer_test.go, executor.go', status: 'done' },
      { time: '10:15:28', title: 'Plan', detail: 'แก้ไข regex สำหรับ quoted path และ escape sequence', status: 'done' },
      {
        time: '10:15:31', title: 'Edit File', detail: 'internal/turn/infer.go', status: 'active',
        change: {
          items: [
            'ปรับ regex ให้รองรับ "quoted path"',
            'รองรับ escape sequence เช่น \\n, \\t, \\\\n',
            'เพิ่ม test case สำหรับ edge cases',
          ],
          footer: 'Lines changed: 42 (+28 −14)',
          badge: 'Editing…',
        },
      },
      { time: '10:15:23', title: 'Run Tests', detail: 'go test ./internal/turn', status: 'wait' },
      { time: '10:15:23', title: 'Verify', detail: 'ตรวจสอบผล test และ lint', status: 'wait' },
      { time: '10:15:23', title: 'Summary', detail: 'สรุปการเปลี่ยนแปลง', status: 'wait' },
    ],
  },

  changedFiles: [
    { path: 'internal/turn/infer.go', status: 'M' },
  ],

  diff: {
    file: 'internal/turn/infer.go',
    hunk: '@@ −245,14 +245,28 @@ func parsePath(input string) (string, error) {',
    lines: [
      { ln: 245, kind: 'ctx', text: ' // old regex ไม่รองรับ quoted path และ escape' },
      { ln: 246, kind: 'ctx', text: ' // var re = regexp.MustCompile(`^([a-zA-Z0-9_\\-./]+)$`)' },
      { ln: 245, kind: 'add', text: ' // new regex รองรับ quoted path และ escape' },
      { ln: 246, kind: 'add', text: ' var re = regexp.MustCompile(`^("([^"\\]|\\.)*"|\'([^\'\\]|\\.)*\'|' },
      { ln: 247, kind: 'add', text: '     [a-zA-Z0-9_\\-./]+))$`)' },
      { ln: 248, kind: 'ctx', text: '' },
      { ln: 249, kind: 'ctx', text: ' func unescapePath(p string) string {' },
      { ln: 250, kind: 'del', text: ' // เดิม return raw' },
      { ln: 251, kind: 'del', text: ' return p' },
      { ln: 252, kind: 'add', text: ' // รองรับ escape sequence' },
      { ln: 253, kind: 'add', text: ' p = strings.ReplaceAll(p, "\\n", "\n")' },
      { ln: 254, kind: 'add', text: ' p = strings.ReplaceAll(p, "\\t", "\t")' },
      { ln: 255, kind: 'add', text: ' p = strings.ReplaceAll(p, "\\\\", "\\")' },
      { ln: 257, kind: 'add', text: ' return p' },
      { ln: 257, kind: 'ctx', text: ' }' },
    ],
  },

  test: {
    command: 'go test ./internal/turn',
    cases: [
      { name: 'TestParser_QuotedPath', state: 'running' },
      { name: 'TestParser_EscapeSequence', state: 'running' },
      { name: 'TestParser_EdgeCases', state: 'running' },
    ],
  },

  commandHistory: [],
}
