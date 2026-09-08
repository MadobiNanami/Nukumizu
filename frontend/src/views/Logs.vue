<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { LOG_LEVELS } from '../utils/fmt.js';

const MAX_LOGS = 1200;

const logs = ref([]);
const pending = ref([]);
const status = ref('connecting');
const paused = ref(false);
const query = ref('');
const enabled = reactive(Object.fromEntries(LOG_LEVELS.map((l) => [l.key, true])));
let ws = null;
let closedManually = false;
let reconnectTimer = null;

const levelOf = (value) => LOG_LEVELS.find((l) => l.value === value) || { key: 'LOG', value };

const visible = computed(() => {
    const q = query.value.trim().toLowerCase();
    return logs.value.filter((m) => {
        if (!enabled[levelOf(m.level).key]) return false;
        if (!q) return true;
        return (
            String(m.content || '').toLowerCase().includes(q) ||
            levelOf(m.level).key.toLowerCase().includes(q)
        );
    });
});

const statusText = computed(() => {
    switch (status.value) {
        case 'open': return { label: 'Connected', cls: 'ok' };
        case 'connecting': return { label: 'Reconnecting', cls: 'warn' };
        default: return { label: 'Disconnected', cls: 'muted' };
    }
});

function wsUrl() {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${proto}//${window.location.host}/api/system/getLogs`;
}

function connect() {
    if (reconnectTimer) {
        clearTimeout(reconnectTimer);
        reconnectTimer = null;
    }
    if (ws) {
        try { ws.close(); } catch { /* ignore */ }
    }
    status.value = 'connecting';

    try {
        ws = new WebSocket(wsUrl());
    } catch {
        scheduleReconnect();
        return;
    }

    ws.onopen = () => {
        status.value = 'open';
    };

    ws.onmessage = (ev) => {
        let msg;
        try {
            msg = JSON.parse(ev.data);
        } catch {
            return;
        }
        if (typeof msg.level !== 'number') return;
        push(msg);
    };

    ws.onclose = () => {
        status.value = 'closed';
        if (!closedManually) scheduleReconnect();
    };

    ws.onerror = () => {
        try { ws.close(); } catch { /* ignore */ }
    };
}

function scheduleReconnect() {
    if (closedManually) return;
    reconnectTimer = setTimeout(connect, 1800);
}

function push(msg) {
    if (paused.value) {
        pending.value.push(msg);
        if (pending.value.length > MAX_LOGS) pending.value.shift();
        return;
    }
    logs.value.unshift(msg);
    if (logs.value.length > MAX_LOGS) logs.value.pop();
}

function togglePause() {
    paused.value = !paused.value;
    if (!paused.value) {
        const buffered = pending.value.splice(0);
        logs.value.unshift(...buffered);
        if (logs.value.length > MAX_LOGS) logs.value.length = MAX_LOGS;
    }
}

function clearLogs() {
    logs.value = [];
    pending.value = [];
}

function exportLogs() {
    const lines = visible.value
        .map((m) => `[${m.timestamp}] [${levelOf(m.level).key.padEnd(5)}] ${m.content}`)
        .join('\n');
    const blob = new Blob([lines], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `nukumizu-logs-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.log`;
    a.click();
    URL.revokeObjectURL(url);
}

function toggleLevel(key) {
    enabled[key] = !enabled[key];
}

onMounted(connect);
onBeforeUnmount(() => {
    closedManually = true;
    if (reconnectTimer) clearTimeout(reconnectTimer);
    if (ws) ws.close();
});
</script>

<template>
    <section class="page">
        <header class="page-head">
            <div>
                <p class="eyebrow">Telemetry</p>
                <h1>System logs</h1>
                <p class="lead">Live log stream from the server. Newest entries appear first.</p>
            </div>
            <div class="head-actions">
                <span class="conn" :class="'conn-' + statusText.cls">
                    <span class="dot" />{{ statusText.label }}
                </span>
                <button class="btn btn-ghost" title="Reconnect" @click="connect"><i class="fas fa-rotate-right" /></button>
                <button class="btn btn-ghost" title="Export visible logs" @click="exportLogs"><i class="fas fa-download" /></button>
                <button class="btn btn-ghost" title="Clear view" @click="clearLogs"><i class="fas fa-eraser" /></button>
            </div>
        </header>

        <div class="toolbar">
            <div class="search">
                <i class="fas fa-magnifying-glass icon" />
                <input v-model="query" type="text" placeholder="Filter logs…" />
            </div>

            <div class="level-chips">
                <button
                    v-for="l in LOG_LEVELS"
                    :key="l.key"
                    class="chip"
                    :class="['lv-' + l.key.toLowerCase(), { off: !enabled[l.key] }]"
                    @click="toggleLevel(l.key)"
                >
                    {{ l.key }}
                </button>
            </div>

            <span class="spacer" />

            <button class="btn" :class="paused ? 'btn-primary' : 'btn-ghost'" @click="togglePause">
                <i class="fas" :class="paused ? 'fa-play' : 'fa-pause'" />
                {{ paused ? `Resume${pending.length ? ` (${pending.length})` : ''}` : 'Pause' }}
            </button>
        </div>

        <div class="card log-card">
            <div v-if="visible.length === 0" class="empty">
                <i class="fas fa-terminal e-icon" />
                <p>No matching log entries.</p>
            </div>
            <div v-else class="log-list">
                <div v-for="(m, i) in visible" :key="m.timestamp + '-' + i" class="log-line">
                    <span class="t mono">{{ m.timestamp }}</span>
                    <span class="lv mono" :class="'lv-' + levelOf(m.level).key.toLowerCase()">
                        {{ levelOf(m.level).key }}
                    </span>
                    <span class="msg">{{ m.content }}</span>
                </div>
            </div>
        </div>
    </section>
</template>

<style scoped>
.conn {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    font-size: 12.5px;
    font-weight: 600;
    padding: 5px 12px;
    border-radius: var(--r-pill);
    border: 1px solid var(--line);
}

.conn-ok { color: var(--ok); }
.conn-ok .dot { background: var(--ok); }
.conn-warn { color: var(--warn); }
.conn-warn .dot { background: var(--warn); }
.conn-muted { color: var(--text-3); }
.conn-muted .dot { background: var(--muted); }

.level-chips { display: flex; gap: 6px; }

.chip { cursor: pointer; transition: opacity 0.15s ease; }
.chip.off { opacity: 0.38; text-decoration: line-through; }

.lv-debug { color: var(--log-debug); }
.lv-info { color: var(--log-info); }
.lv-warn { color: var(--log-warn); }
.lv-error { color: var(--log-error); }
.lv-fatal { color: var(--log-fatal); }

.log-card { overflow: hidden; }

.log-list {
    max-height: calc(100vh - 300px);
    overflow-y: auto;
    font-family: var(--font-mono);
}

.log-line {
    display: grid;
    grid-template-columns: 170px 72px 1fr;
    gap: 14px;
    padding: 7px 16px;
    font-size: 12.5px;
    line-height: 1.45;
    border-bottom: 1px solid var(--line);
    align-items: baseline;
}

.log-line:hover { background: var(--surface-2); }

.log-line .t { color: var(--text-3); white-space: nowrap; }

.log-line .lv {
    font-weight: 700;
    font-size: 11px;
    letter-spacing: 0.04em;
    padding: 1px 8px;
    border-radius: var(--r-pill);
    background: var(--surface-3);
    text-align: center;
}

.log-line .lv.lv-debug { background: var(--ok-soft); }
.log-line .lv.lv-info { background: var(--accent-soft); }
.log-line .lv.lv-warn { background: var(--warn-soft); }
.log-line .lv.lv-error { background: var(--bad-soft); }
.log-line .lv.lv-fatal { background: var(--bad-soft); }

.log-line .msg { color: var(--text); word-break: break-word; white-space: pre-wrap; }
</style>
