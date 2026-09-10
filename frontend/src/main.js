import { createApp } from 'vue';
import App from './App.vue';
import router from './router/index.js';
import { initTheme } from './utils/theme.js';
import { isLoggedIn } from './utils/auth.js';
import { loadDebugMode } from './utils/runtime.js';
import '@fortawesome/fontawesome-free/css/all.min.css';
import './styles/theme.css';
import './styles/ui.css';

initTheme();

// A persisted session can fetch the runtime flags right away; a fresh visitor
// has no token yet and picks them up after signing in.
if (isLoggedIn()) {
    loadDebugMode();
}

createApp(App).use(router).mount('#app');
