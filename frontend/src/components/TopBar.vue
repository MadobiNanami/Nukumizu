<script setup>
import { computed, ref } from 'vue';
import { useRouter } from 'vue-router';
import { auth, clearSession } from '../utils/auth.js';
import { currentTheme, toggleTheme } from '../utils/theme.js';
import { toast } from '../utils/toast.js';

const router = useRouter();

const isDark = ref(currentTheme() === 'dark');
const user = computed(() => auth.user || {});
const initial = computed(() => ((user.value.username || '?').slice(0, 1) || '?').toUpperCase());

function onToggleTheme() {
    isDark.value = toggleTheme() === 'dark';
}

function logout() {
    clearSession();
    toast.info('Signed out');
    router.push({ name: 'Login' });
}
</script>

<template>
    <header class="topbar">
        <div class="brand">
            <span class="mark">
                <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 2 3 6.5v3.1C3 15.2 6.9 20 12 21c5.1-1 9-5.8 9-11.4V6.5L12 2Zm0 7.2V19c-3.7-.7-6.6-4.4-6.6-8.2V7.6L12 4.4l6.6 3.2v3.2c0 1.9-.5 3.7-1.4 5.1H12Z" fill="currentColor"/></svg>
            </span>
            <span class="wordmark">
                <strong>Nukumizu</strong>
                <small>Console</small>
            </span>
        </div>

        <div class="topbar-right">
            <button class="icon-btn" :title="isDark ? 'Switch to light' : 'Switch to dark'" @click="onToggleTheme">
                <i class="fas" :class="isDark ? 'fa-sun' : 'fa-moon'" />
            </button>

            <div class="user-chip">
                <span class="avatar">{{ initial }}</span>
                <div class="who">
                    <span class="name">{{ user.username || '—' }}</span>
                    <span class="level">{{ user.level || 'admin' }}</span>
                </div>
            </div>

            <button class="icon-btn" title="Sign out" @click="logout">
                <i class="fas fa-arrow-right-from-bracket" />
            </button>
        </div>
    </header>
</template>

<style scoped>
.topbar {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    height: 52px;
    z-index: 40;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 18px;
    background: var(--surface);
    border-bottom: 1px solid var(--line);
    box-shadow: var(--shadow-1);
}

.brand {
    display: flex;
    align-items: center;
    gap: 11px;
}

.mark {
    width: 30px;
    height: 30px;
    border-radius: 8px;
    display: grid;
    place-items: center;
    background: linear-gradient(150deg, var(--accent), var(--accent-strong));
    color: #fff;
    box-shadow: 0 3px 10px -3px color-mix(in srgb, var(--accent) 60%, transparent);
}

.mark svg {
    width: 19px;
    height: 19px;
}

.wordmark {
    display: flex;
    align-items: baseline;
    gap: 8px;
    line-height: 1;
}

.wordmark strong {
    font-family: var(--font-display);
    font-size: 17px;
    font-weight: 700;
    letter-spacing: -0.02em;
}

.wordmark small {
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: var(--text-3);
}

.topbar-right {
    display: flex;
    align-items: center;
    gap: 10px;
}

.icon-btn {
    width: 34px;
    height: 34px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    color: var(--text-2);
    font-size: 15px;
    transition: background 0.15s ease, color 0.15s ease;
}

.icon-btn:hover {
    background: var(--surface-3);
    color: var(--text);
}

.user-chip {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 5px 6px 5px 5px;
    border-radius: var(--r-pill);
    border: 1px solid var(--line);
    background: var(--surface-2);
}

.avatar {
    width: 30px;
    height: 30px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    background: var(--accent-soft);
    color: var(--accent);
    font-weight: 700;
    font-size: 14px;
}

.who {
    display: flex;
    flex-direction: column;
    line-height: 1.15;
    padding-right: 6px;
}

.who .name {
    font-size: 13px;
    font-weight: 600;
}

.who .level {
    font-size: 11px;
    color: var(--text-3);
    text-transform: uppercase;
    letter-spacing: 0.06em;
}
</style>
