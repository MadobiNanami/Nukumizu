import { createRouter, createWebHistory } from 'vue-router';
import { isLoggedIn } from '../utils/auth.js';

const routes = [
    {
        path: '/login',
        name: 'Login',
        component: () => import('../views/Login.vue'),
        meta: { public: true }
    },
    {
        path: '/',
        component: () => import('../views/Layout.vue'),
        redirect: { name: 'Overview' },
        children: [
            {
                path: 'overview',
                name: 'Overview',
                component: () => import('../views/Overview.vue'),
                meta: { title: 'Nodes' }
            },
            {
                path: 'trusted',
                name: 'Trusted',
                component: () => import('../views/Trusted.vue'),
                meta: { title: 'Bot trust' }
            },
            {
                path: 'settings',
                name: 'Settings',
                component: () => import('../views/Settings.vue'),
                meta: { title: 'Settings' }
            },
            {
                path: 'logs',
                name: 'Logs',
                component: () => import('../views/Logs.vue'),
                meta: { title: 'Logs' }
            }
        ]
    },
    { path: '/:pathMatch(.*)*', redirect: '/' }
];

const router = createRouter({
    history: createWebHistory(),
    routes
});

router.beforeEach((to) => {
    if (!to.meta.public && !isLoggedIn()) {
        return { name: 'Login', query: { redirect: to.fullPath } };
    }
    if (to.name === 'Login' && isLoggedIn()) {
        return { name: 'Overview' };
    }
    return true;
});

export default router;
