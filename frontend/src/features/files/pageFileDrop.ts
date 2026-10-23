// Listen above the card so drops on the background and floating controls work too.
export function listenForPageFileDrops(
  target: Window,
  onFiles: (files: File[]) => void,
  onDragging: (dragging: boolean) => void,
) {
  let depth = 0
  const reset = () => {
    depth = 0
    onDragging(false)
  }
  const enter = (event: DragEvent) => {
    if (!event.dataTransfer?.types.includes('Files')) return
    event.preventDefault()
    depth += 1
    onDragging(true)
  }
  const over = (event: DragEvent) => {
    if (!event.dataTransfer?.types.includes('Files')) return
    event.preventDefault()
    event.dataTransfer.dropEffect = 'copy'
  }
  const leave = () => {
    depth = Math.max(0, depth - 1)
    if (depth === 0) onDragging(false)
  }
  const drop = (event: DragEvent) => {
    reset()
    const files = Array.from(event.dataTransfer?.files ?? [])
    if (!event.dataTransfer?.types.includes('Files') && files.length === 0) return
    event.preventDefault()
    if (files.length > 0) onFiles(files)
  }

  target.addEventListener('dragenter', enter, true)
  target.addEventListener('dragover', over, true)
  target.addEventListener('dragleave', leave, true)
  target.addEventListener('drop', drop, true)
  target.addEventListener('dragend', reset)
  target.addEventListener('blur', reset)
  return () => {
    target.removeEventListener('dragenter', enter, true)
    target.removeEventListener('dragover', over, true)
    target.removeEventListener('dragleave', leave, true)
    target.removeEventListener('drop', drop, true)
    target.removeEventListener('dragend', reset)
    target.removeEventListener('blur', reset)
  }
}
