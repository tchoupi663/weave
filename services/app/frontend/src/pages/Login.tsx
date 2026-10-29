import { useState } from 'react'
import type { FormEvent } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { api } from '../api'
import { setSession } from '../auth'

export function Login() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const nextParam = params.get('next') || '/products'
  const next = nextParam.startsWith('/') ? nextParam : '/products'

  const [username, setUsername] = useState('user0001')
  const [password, setPassword] = useState('password')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      const result = await api.login(username, password)
      setSession(result.token, username)
      navigate(next, { replace: true })
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Could not sign in.')
      setBusy(false)
    }
  }

  return (
    <section className="panel narrow">
      <p className="eyebrow">Account</p>
      <h1>Sign in</h1>
      <p className="muted">Use a seeded demo account to check out.</p>
      <form onSubmit={submit}>
        <label className="field">
          <span>Username</span>
          <input
            autoComplete="username"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
          />
        </label>
        <label className="field">
          <span>Password</span>
          <input
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </label>
        {error && <p className="error">{error}</p>}
        <button className="btn btn-block" type="submit" disabled={busy}>
          {busy ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
      <p className="fineprint">
        Demo account: <code>user0001</code> / <code>password</code>. Accounts run from user0001 to
        user2000.
      </p>
    </section>
  )
}
