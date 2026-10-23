import type { ApiError } from '../../api/client'

interface StatusMessageProps {
  error?: ApiError | null
  message?: string | null
}

export default function StatusMessage({ error, message }: StatusMessageProps) {
  if (!error && !message) return null

  if (error) {
    return (
      <div className="status-message status-message--error" role="alert">
        <p>{error.message}</p>
        {error.fields && (
          <ul>
            {Object.entries(error.fields).map(([field, detail]) => (
              <li key={field}>{field}: {detail}</li>
            ))}
          </ul>
        )}
      </div>
    )
  }

  return <div className="status-message status-message--success" role="status">{message}</div>
}
