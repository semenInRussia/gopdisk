import { useId, useLayoutEffect, useRef, useState } from 'react'
import Icon from '../../components/Icon'

interface Props {
  value: string
  onChange: (value: string) => void
}

export default function FileSearch({ value, onChange }: Props) {
  const [expanded, setExpanded] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const id = useId()

  useLayoutEffect(() => {
    if (expanded) input.current?.focus()
  }, [expanded])

  function close() {
    onChange('')
    setExpanded(false)
    trigger.current?.focus()
  }

  return (
    <div className={`file-search${expanded ? ' file-search--open' : ''}`} role="search" aria-label="Поиск файлов" onBlur={(event) => {
      if (!event.currentTarget.contains(event.relatedTarget) && !value) setExpanded(false)
    }} onKeyDown={(event) => {
      if (expanded && event.key === 'Escape') {
        event.preventDefault()
        event.stopPropagation()
        close()
      }
    }}>
      <input ref={input} id={id} className="p-field" type="search" placeholder="Поиск файлов" aria-label="Поиск файлов по имени" autoComplete="off"
        disabled={!expanded} aria-hidden={!expanded} value={value} onChange={(event) => onChange(event.target.value)} />
      <button ref={trigger} className="p-button p-button-borderless p-button-icon-only file-search-trigger" type="button" aria-label="Открыть поиск файлов" aria-expanded={expanded} aria-controls={id}
        onClick={() => {
          if (expanded) input.current?.focus()
          else setExpanded(true)
        }}>
        <Icon name="search" />
      </button>
      {expanded && <button className="p-button p-button-borderless p-button-icon-only file-search-close" type="button" aria-label="Закрыть поиск" onClick={close}><Icon name="close" /></button>}
    </div>
  )
}
