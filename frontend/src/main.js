import { createApp } from 'vue';
import App from './App.vue';
import router from './router/index.js';
import { initTheme } from './utils/theme.js';
import '@fortawesome/fontawesome-free/css/all.min.css';
import './styles/theme.css';
import './styles/ui.css';

initTheme();

createApp(App).use(router).mount('#app');
