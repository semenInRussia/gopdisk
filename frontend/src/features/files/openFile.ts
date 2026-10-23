import { apiBlobRequest } from '../../api/client'

export async function openFile(id: string, token: string | null): Promise<void> {
  // Open during the click so the browser accepts the new tab.
  const tab = window.open('about:blank', '_blank')
  if (!tab) {
    throw new Error('Браузер заблокировал новую вкладку. Разрешите всплывающие окна для этого сайта.')
  }
  tab.opener = null
  // The content tab does not need the copy of the application's session.
  tab.sessionStorage.clear()

  let url: string | undefined
  try {
    const content = await apiBlobRequest(`/v1/files/${encodeURIComponent(id)}/content`, {}, token)
    if (tab.closed) return
    url = URL.createObjectURL(content)
    tab.location.replace(url)
  } catch (error) {
    if (url) URL.revokeObjectURL(url)
    tab.close()
    throw error
  }

  // Keep the URL alive for media playback and seeking until the tab is closed.
  const openedURL = url
  const timer = window.setInterval(() => {
    if (tab.closed) {
      URL.revokeObjectURL(openedURL)
      window.clearInterval(timer)
    }
  }, 1000)
}
