import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
// i18n 必须先于 App import：模块加载即完成 i18next 初始化（语言从
// localStorage 读出），保证首次渲染前语言就绪、无首帧闪错。
import '@/i18n'
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
