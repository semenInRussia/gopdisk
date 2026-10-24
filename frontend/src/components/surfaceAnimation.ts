import type { MouseEvent } from 'react'

export interface SurfaceOrigin {
  x: number
  y: number
}

export function activationPoint(event: MouseEvent<HTMLElement>): SurfaceOrigin {
  if (event.detail !== 0) return { x: event.clientX, y: event.clientY }
  const rect = event.currentTarget.getBoundingClientRect()
  return { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 }
}

export function collapsedSurfaceFrame(element: HTMLElement, origin: SurfaceOrigin): Keyframe {
  const rect = element.getBoundingClientRect()
  const x = origin.x - rect.left - rect.width / 2
  const y = origin.y - rect.top - rect.height / 2
  return { opacity: 0, transform: `translate(${x}px, ${y}px) scale(0.08)`, transformOrigin: 'center' }
}
