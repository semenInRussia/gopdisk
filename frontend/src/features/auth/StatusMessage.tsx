import type { ApiError } from '../../api/client'
import Icon from '../../components/Icon'

const fieldLabels: Record<string, string> = {
  name: 'Имя пользователя', email: 'Электронная почта', password: 'Пароль',
}

interface StatusMessageProps {
  error?: ApiError | null
  message?: string | null
}

export default function StatusMessage({ error, message }: StatusMessageProps) {
  if (!error && !message) return null

  if (error) {
    return (
      <div className="p-glass status-message status-message--error" role="alert">
        <Icon name="warning" />
        <div>
          <p>{error.message}</p>
          {error.fields && (
            <ul>
              {Object.entries(error.fields).map(([field, detail]) => (
                <li key={field}>{fieldLabels[field] ?? field}: {detail}</li>
              ))}
            </ul>
          )}
        </div>
      </div>
    )
  }

  return <div className="p-glass status-message status-message--success" role="status"><Icon name="check" /><p>{message}</p></div>
}
