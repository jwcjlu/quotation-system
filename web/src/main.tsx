import { createRoot } from 'react-dom/client'
import './style.css'
import App from './App'

// 不使用 StrictMode：React 18 开发环境下 StrictMode 会双重挂载并让 useEffect 整段再跑一遍，
// 凡在 effect 里发起的 fetch 都会成对出现，易误判为后端/代理问题。
createRoot(document.getElementById('root')!).render(<App />)
