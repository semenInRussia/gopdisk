import type { ReactNode } from 'react'

export default function ToolbarGroup({ label, className = '', children }: { label: string; className?: string; children: ReactNode }) {
  return <div className={`p-toolbar-group ${className}`} role="toolbar" aria-label={label} onKeyDown={(event) => {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
    const buttons = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')]
      .filter((button) => window.getComputedStyle(button).display !== 'none')
    if (!buttons.length) return
    event.preventDefault()
    const index = buttons.indexOf(document.activeElement as HTMLButtonElement)
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1
      : (index + (event.key === 'ArrowRight' ? 1 : -1) + buttons.length) % buttons.length
    buttons[next].focus()
  }}>{children}</div>
}
