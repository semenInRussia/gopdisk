import type { IconName } from '../../components/Icon'

export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 Б'
  const units = ['Б', 'КиБ', 'МиБ', 'ГиБ', 'ТиБ']
  const unit = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / 1024 ** unit).toLocaleString('ru-RU', { maximumFractionDigits: unit === 0 ? 0 : 1 })} ${units[unit]}`
}

export function describeFile(type: string, name: string): { icon: IconName; label: string } {
  if (type.startsWith('image/')) return { icon: 'image', label: 'Изображение' }
  if (type.startsWith('video/')) return { icon: 'video', label: 'Видео' }
  if (type.startsWith('audio/')) return { icon: 'audio', label: 'Аудио' }
  if (type === 'application/pdf') return { icon: 'file', label: 'Документ PDF' }
  if (type.startsWith('text/')) return { icon: 'file', label: 'Текстовый документ' }
  const extension = name.includes('.') ? name.split('.').pop()?.toUpperCase() : null
  return { icon: 'file', label: extension ? `Файл ${extension}` : 'Файл' }
}
