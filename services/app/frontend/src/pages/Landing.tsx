import { Link } from 'react-router-dom'

export function Landing() {
  return (
    <section className="hero">
      <p className="eyebrow">Weave Target App</p>
      <h1>
        Everyday goods,
        <br />
        simulated at scale.
      </h1>
      <p className="lede">
        A deliberately small storefront built to generate realistic traffic, topology, and edge
        telemetry for Weave.
      </p>
      <div className="hero-actions">
        <Link className="btn" to="/products">
          Browse products
        </Link>
        <Link className="btn btn-ghost" to="/login">
          Sign in
        </Link>
      </div>
      <div className="hero-grid" aria-hidden="true">
        <span />
        <span />
        <span />
        <span />
      </div>
    </section>
  )
}
