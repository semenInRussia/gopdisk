import { useLayoutEffect, useRef, useState, type RefObject } from 'react'
import { collapsedSurfaceFrame, type SurfaceOrigin } from './surfaceAnimation'

export function useSurfaceTransition<T extends HTMLElement>(ref: RefObject<T | null>, open: boolean, origin: SurfaceOrigin, onHidden?: () => void): boolean {
  const [present, setPresent] = useState(open)
  const interrupted = useRef<Keyframe | null>(null)
  const hiddenCallback = useRef(onHidden)
  hiddenCallback.current = onHidden

  useLayoutEffect(() => {
    if (open && !present) { setPresent(true); return }
    const element = ref.current
    if (!present || !element) return
    if (element.tagName === 'DIALOG' && !(element as unknown as HTMLDialogElement).open) {
      (element as unknown as HTMLDialogElement).showModal()
    }
    const finishClose = () => {
      hiddenCallback.current?.()
      setPresent(false)
    }
    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)')
    if (reducedMotion.matches) {
      interrupted.current = null
      if (!open) finishClose()
      return
    }
    const collapsed = collapsedSurfaceFrame(element, origin)
    const expanded: Keyframe = { opacity: 1, transform: 'none', transformOrigin: 'center' }
    const from = interrupted.current ?? (open ? collapsed : expanded)
    interrupted.current = null
    const animation = element.animate([from, open ? expanded : collapsed], {
      duration: open ? 420 : 300,
      easing: open ? 'cubic-bezier(0.16, 1, 0.3, 1)' : 'cubic-bezier(0.4, 0, 0.2, 1)',
      fill: 'both',
    })
    animation.onfinish = () => {
      if (open) animation.cancel()
      else finishClose()
    }
    const finish = () => animation.finish()
    reducedMotion.addEventListener('change', finish)
    return () => {
      if (animation.playState === 'running') {
        const style = getComputedStyle(element)
        interrupted.current = { opacity: style.opacity, transform: style.transform, transformOrigin: 'center' }
      }
      animation.onfinish = null
      animation.cancel()
      reducedMotion.removeEventListener('change', finish)
    }
  }, [open, present, origin, ref])

  return present
}
