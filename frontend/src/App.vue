<script setup>
import { onBeforeUnmount, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { toasts } from './utils/toast.js';

const router = useRouter();

const glyph = { success: '✓', error: '✕', info: 'i', warn: '!' };

function onAuthExpired() {
    if (router.currentRoute.value.name !== 'Login') {
        router.push({ name: 'Login' });
    }
}

onMounted(() => window.addEventListener('auth:expired', onAuthExpired));
onBeforeUnmount(() => window.removeEventListener('auth:expired', onAuthExpired));
</script>

<template>
    <router-view />
    <div class="toasts" aria-live="polite">
        <div
            v-for="t in toasts"
            :key="t.id"
            class="toast"
            :class="[t.type, { leaving: t.leaving }]"
            role="status"
        >
            <span class="t-icon">{{ glyph[t.type] || 'i' }}</span>
            <span class="t-msg">{{ t.message }}</span>
        </div>
    </div>
</template>
