export function animateFileDeletion(element: HTMLElement | null): Promise<void> {
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)')
  if (!element || reducedMotion.matches || typeof element.animate !== 'function') return Promise.resolve()

  element.classList.add('file-removing')
  const animation = element.animate([
    { opacity: 1, transform: 'none', offset: 0 },
    { opacity: 1, transform: 'translateY(12px) rotate(2deg)', offset: 0.35 },
    { opacity: 0, transform: 'translateY(110px) rotate(9deg) scale(0.92)', offset: 1 },
  ], { duration: 380, easing: 'cubic-bezier(0.4, 0, 1, 1)', fill: 'forwards' })

  return new Promise((resolve) => {
    const finish = () => animation.finish()
    const done = () => {
      reducedMotion.removeEventListener('change', finish)
      window.removeEventListener('resize', finish)
      resolve()
    }
    animation.onfinish = done
    animation.oncancel = done
    reducedMotion.addEventListener('change', finish)
    window.addEventListener('resize', finish)
  })
}
