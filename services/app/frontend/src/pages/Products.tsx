import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { formatPrice } from '../cart'
import type { Product } from '../types'

function artStyle(index: number) {
  const hue = (index * 47 + 18) % 360
  return {
    background: `linear-gradient(135deg, hsl(${hue} 58% 74%), hsl(${(hue + 42) % 360} 48% 52%))`,
  }
}

export function Products() {
  const [products, setProducts] = useState<Product[] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    api
      .products()
      .then((items) => {
        if (!cancelled) setProducts(items)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Could not load products.')
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (error) {
    return (
      <section className="panel">
        <h1>Products</h1>
        <p className="error">{error}</p>
        <Link className="btn" to="/">
          Back home
        </Link>
      </section>
    )
  }

  if (!products) {
    return (
      <section className="panel">
        <h1>Products</h1>
        <p className="muted">Loading the shelf…</p>
      </section>
    )
  }

  return (
    <section>
      <div className="section-head">
        <div>
          <p className="eyebrow">Catalog</p>
          <h1>Twenty useful things</h1>
        </div>
        <span className="count">{products.length} items</span>
      </div>
      <div className="product-grid">
        {products.map((product, index) => (
          <Link className="product-card" to={`/products/${product.id}`} key={product.id}>
            <div className="product-art" style={artStyle(index)}>
              <span>{product.name.slice(-2)}</span>
            </div>
            <div className="product-body">
              <span className="product-id">{product.id}</span>
              <h2>{product.name}</h2>
              <span className="price">{formatPrice(product.price)}</span>
            </div>
          </Link>
        ))}
      </div>
    </section>
  )
}
