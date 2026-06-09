import ReactDOM from 'react-dom/client'

import { App } from './app'

// Apply server-injected theme CSS variables before React mounts
// to avoid any flash of default colors.
function applyServerTheme() {
    const getMeta = (name: string) =>
        document.querySelector<HTMLMetaElement>(`meta[name="${name}"]`)?.content ?? ''

    const root = document.documentElement
    const bg = getMeta('rwsp-theme-bg')
    const accentL = getMeta('rwsp-theme-accent-l')
    const accentR = getMeta('rwsp-theme-accent-r')
    const primary = getMeta('rwsp-theme-primary')

    if (bg) root.style.setProperty('--rwsp-bg', bg)
    if (accentL) root.style.setProperty('--rwsp-accent-l', accentL)
    if (accentR) root.style.setProperty('--rwsp-accent-r', accentR)
    if (primary) root.dataset.primaryColor = primary
}

applyServerTheme()

const domRoot = ReactDOM.createRoot(document.getElementById('root')!)
domRoot.render(<App />)
