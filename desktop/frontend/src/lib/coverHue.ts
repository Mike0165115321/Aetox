// The colour a named thing wears in a gallery.
//
// One hash, three galleries: presets (Settings), เอเจน (Office) and โปรเจกต์
// (Projects). It lived twice as an identical private function before this file,
// with a comment in each copy saying it had to match the other — which is the
// shape of a rule nothing enforces. A third copy would have been the point
// where they started to drift.
//
// Stable by construction: the name is the only input, so a thing keeps its
// colour for life and moving it between galleries does not repaint it.
export function coverHue(name: string): number {
  let h = 0
  for (const ch of name) h = (h * 31 + ch.codePointAt(0)!) % 360
  return h
}
