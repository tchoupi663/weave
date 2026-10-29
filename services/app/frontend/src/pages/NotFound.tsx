import { Link } from 'react-router-dom'

export function NotFound() {
  return (
    <section className="panel narrow">
      <p className="eyebrow">404</p>
      <h1>Page not found</h1>
      <p className="muted">That route is not part of the storefront.</p>
      <Link className="btn" to="/">
        Back home
      </Link>
    </section>
  )
}
