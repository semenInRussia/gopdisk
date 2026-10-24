import { useEffect, type RefObject } from 'react'

// Keep the sheet above a software keyboard without resizing the page behind it.
export function useDialogViewport(ref: RefObject<HTMLDialogElement | null>) {
  useEffect(() => {
    const viewport = window.visualViewport
    const element = ref.current
    if (!viewport || !element) return
    const measure = () => {
      element.style.setProperty('--permissions-viewport-height', `${viewport.height}px`)
      element.style.setProperty('--permissions-keyboard-offset', `${Math.max(0, window.innerHeight - viewport.height - viewport.offsetTop)}px`)
    }
    measure()
    viewport.addEventListener('resize', measure)
    viewport.addEventListener('scroll', measure)
    return () => {
      viewport.removeEventListener('resize', measure)
      viewport.removeEventListener('scroll', measure)
    }
  }, [ref])
}
