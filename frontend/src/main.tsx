import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import './styles/tokens.css'
import './styles/global.css'
import './styles/chat-markdown.css'

const rootEl = document.getElementById('root')
if (!rootEl) throw new Error('Missing #root element in index.html')
createRoot(rootEl).render(
  <StrictMode>
    <App />
  </StrictMode>
)
