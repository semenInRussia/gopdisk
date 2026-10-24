export const TOKEN_KEY = 'gopherdisk.authenticationToken'
const MIGRATION_KEY = 'gopherdisk.sharedSession'

export function readSessionToken(): string | null {
  let token = window.localStorage.getItem(TOKEN_KEY)
  // Migrate an existing tab session once; never revive it after a shared logout.
  if (!window.localStorage.getItem(MIGRATION_KEY)) {
    token ??= window.sessionStorage.getItem(TOKEN_KEY)
    if (token) window.localStorage.setItem(TOKEN_KEY, token)
    window.localStorage.setItem(MIGRATION_KEY, '1')
  }
  window.sessionStorage.removeItem(TOKEN_KEY)
  return token
}

export function saveSessionToken(token: string | null): void {
  window.localStorage.setItem(MIGRATION_KEY, '1')
  window.sessionStorage.removeItem(TOKEN_KEY)
  if (token) window.localStorage.setItem(TOKEN_KEY, token)
  else window.localStorage.removeItem(TOKEN_KEY)
}
