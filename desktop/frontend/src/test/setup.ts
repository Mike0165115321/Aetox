// What jsdom is missing that the app actually uses.
//
// Only genuine environment gaps belong here — things a real browser has and
// jsdom does not. Anything about how *our* code behaves belongs in a test, not
// in a shim that every test silently inherits.

// The Web Animations API. jsdom does not implement `element.animate`, and
// Svelte 5 runs every `transition:` through it — so the first transition to
// appear in a component under test threw `element.animate is not a function`
// from inside Svelte, on a timer, outside any test's call stack.
//
// That shape is worth naming: vitest reported it as an *unhandled error*, not
// as a failing test. Every assertion still passed, the summary still read
// "56 passed (56)", and the run still exited 1 — so a green-looking local run
// piped through `tail` hid it completely, and CI was the first thing to say so
// out loud (2026-08-16).
//
// The stub reports a finished animation and calls `onfinish` as soon as Svelte
// attaches one, which is what makes an outro complete and the element leave the
// document — the same end state a real browser reaches, minus the frames in
// between. Tests that care about a transition's *timing* would need a real
// clock; none do, and if one ever does it should say so itself rather than
// change this for everyone.
if (typeof Element !== 'undefined' && !Element.prototype.animate) {
  Element.prototype.animate = function (): Animation {
    let finishHandler: ((this: Animation, ev: AnimationPlaybackEvent) => unknown) | null = null
    const animation = {
      currentTime: 0,
      startTime: 0,
      playbackRate: 1,
      playState: 'finished' as AnimationPlayState,
      effect: null,
      timeline: null,
      id: '',
      pending: false,
      ready: Promise.resolve(),
      finished: Promise.resolve(),
      oncancel: null,
      onremove: null,
      get onfinish() {
        return finishHandler
      },
      set onfinish(handler) {
        finishHandler = handler
        if (handler) queueMicrotask(() => handler.call(animation as Animation, {} as AnimationPlaybackEvent))
      },
      cancel() {},
      play() {},
      pause() {},
      reverse() {},
      persist() {},
      commitStyles() {},
      updatePlaybackRate() {},
      finish() {
        finishHandler?.call(animation as Animation, {} as AnimationPlaybackEvent)
      },
      addEventListener() {},
      removeEventListener() {},
      dispatchEvent() {
        return true
      },
    }
    return animation as unknown as Animation
  } as Element['animate']
}
