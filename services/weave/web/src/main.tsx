import '@fontsource-variable/inter'
import '@fontsource/fragment-mono/400.css'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import { installScrollBounce } from './scrollBounce'
import './styles.css'

// The rubber band is page-level behaviour, not React state: it attaches once
// for the life of the console.
installScrollBounce(() => Boolean(document.querySelector('.graph-shell:hover')))

createRoot(document.getElementById('root') as HTMLElement).render(
  <StrictMode>
    <>
      <App />
    </>
  </StrictMode>,
)
