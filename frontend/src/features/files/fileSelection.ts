export function selectFileIds(
  current: string[], ordered: string[], id: string, anchor: string | null,
  modifiers: { toggle: boolean; range: boolean },
): string[] {
  if (!ordered.includes(id)) return current
  if (modifiers.range && anchor && ordered.includes(anchor)) {
    const start = ordered.indexOf(anchor)
    const end = ordered.indexOf(id)
    const range = ordered.slice(Math.min(start, end), Math.max(start, end) + 1)
    return modifiers.toggle ? [...new Set([...current, ...range])] : range
  }
  if (modifiers.toggle) return current.includes(id) ? current.filter((item) => item !== id) : [...current, id]
  return [id]
}

export function intersects(a: { left: number; top: number; right: number; bottom: number }, b: { left: number; top: number; right: number; bottom: number }): boolean {
  return a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top
}
