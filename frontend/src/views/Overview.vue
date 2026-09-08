<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { serverApi, settingsApi } from '../api/index.js';
import { toast } from '../utils/toast.js';
import { formatBytes, shortUUID } from '../utils/fmt.js';
import Toggle from '../components/Toggle.vue';

function percent(m) {
    return m && m.total ? Math.min(100, Math.max(0, Math.round((m.used / m.total) * 100))) : 0;
}

const query = ref('');
const loading = ref(true);
const failed = ref(false);
const entries = ref([]);
const saving = reactive(new Set());
let timer = null;

const filtered = computed(() => {
    const q = query.value.trim().toLowerCase();
    if (!q) return entries.value;
    return entries.value.filter((e) =>
        (e.name || '').toLowerCase().includes(q) || (e.uuid || '').toLowerCase().includes(q)
    );
});

const stats = computed(() => {
    let online = 0;
    for (const e of entries.value) if (e.online) online += 1;
    return { total: entries.value.length, online, offline: entries.value.length - online };
});

function meterClass(pct) {
    if (pct >= 90) return 'bad';
    if (pct >= 75) return 'warn';
    return 'ok';
}

async function load() {
    try {
        const [status, info, nodes] = await Promise.all([
            serverApi.statusAll(),
            serverApi.infoAll(),
            settingsApi.get('bot_node_config')
        ]);
        const nodeConf = (nodes && nodes.config) || {};
        const list = [];
        for (const key of Object.keys(status)) {
            if (key === 'success') continue;
            const s = status[key] || {};
            const infoEntry = info[key] || {};
            const conf = nodeConf[key] || {};
            list.push({
                uuid: s.uuid || key,
                name: s.name || shortUUID(s.uuid || key),
                online: !!s.online,
                report: s.report || null,
                info: infoEntry.info || null,
                notify: conf.enableStatusNotify !== false
            });
        }
        list.sort((a, b) => (a.online === b.online ? a.name.localeCompare(b.name) : b.online ? 1 : -1));
        entries.value = list;
        failed.value = false;
    } catch (e) {
        failed.value = true;
        if (e.status !== 0) toast.error(e.message);
    } finally {
        loading.value = false;
    }
}

async function toggleNotify(entry) {
    const next = !entry.notify;
    const prev = entry.notify;
    entry.notify = next;
    saving.add(entry.uuid);
    try {
        const patch = {};
        patch[entry.uuid] = { enableStatusNotify: next };
        await settingsApi.set('bot_node_config', patch);
        toast.success(`${entry.name}: status notify ${next ? 'on' : 'off'}`);
    } catch (e) {
        entry.notify = prev;
        toast.error('Save failed: ' + e.message);
    } finally {
        saving.delete(entry.uuid);
    }
}

function copyUUID(uuid) {
    if (navigator.clipboard) {
        navigator.clipboard.writeText(uuid).then(() => toast.success('UUID copied'), () => {});
    }
}

onMounted(() => {
    load();
    timer = setInterval(load, 15000);
});

onBeforeUnmount(() => {
    if (timer) clearInterval(timer);
});
</script>

<template>
    <section class="page">
        <header class="page-head">
            <div>
                <p class="eyebrow">Overview</p>
                <h1>Nodes</h1>
                <p class="lead">Monitored servers with live load, online state, and the per-node status-notify switch.</p>
            </div>
            <div class="head-actions">
                <button class="btn btn-ghost" title="Refresh" :disabled="loading" @click="load">
                    <i class="fas fa-rotate" :class="{ 'fa-spin': loading }" />
                </button>
            </div>
        </header>

        <div class="toolbar">
            <div class="search">
                <i class="fas fa-magnifying-glass icon" />
                <input v-model="query" type="text" placeholder="Search name or UUID…" />
            </div>
            <div class="badge badge-accent"><span class="dot" /> {{ stats.total }} total</div>
            <div class="badge badge-online"><span class="dot" /> {{ stats.online }} online</div>
            <div class="badge badge-offline"><span class="dot" /> {{ stats.offline }} offline</div>
            <span class="spacer" />
            <span class="refresh-hint"><i class="fas fa-circle-info" /> Auto-refreshes every 15s</span>
        </div>

        <div v-if="failed && !loading" class="card empty">
            <i class="fas fa-plug-circle-xmark e-icon" />
            <p>Cannot reach the backend service</p>
            <button class="btn btn-primary" @click="load">Retry</button>
        </div>

        <div v-else-if="loading" class="card empty">
            <span class="spinner" />
        </div>

        <div v-else-if="entries.length === 0" class="card empty">
            <i class="fas fa-server e-icon" />
            <p>No monitored nodes yet</p>
        </div>

        <div v-else-if="filtered.length === 0" class="card empty">
            <i class="fas fa-magnifying-glass e-icon" />
            <p>No nodes match “{{ query }}”</p>
        </div>

        <div v-else class="grid-2">
            <article
                v-for="e in filtered"
                :key="e.uuid"
                class="card server-card"
                :class="{ dim: !e.online }"
            >
                <div class="row-1">
                    <span class="sv-icon"><i class="fas fa-server" /></span>
                    <div class="sv-main">
                        <p class="sv-name" :title="e.name">{{ e.name }}</p>
                        <button class="sv-uuid" type="button" :title="'Copy: ' + e.uuid" @click="copyUUID(e.uuid)">
                            {{ shortUUID(e.uuid) }}
                            <i class="fas fa-copy" />
                        </button>
                    </div>
                    <span class="badge" :class="e.online ? 'badge-online' : 'badge-offline'">
                        <span class="dot" />{{ e.online ? 'Online' : 'Offline' }}
                    </span>
                </div>

                <template v-if="e.online && e.report">
                    <div class="meter-row">
                        <span class="mr-label">CPU</span>
                        <div class="meter"><i :class="meterClass(e.report.cpu.usage)" :style="{ width: Math.min(100, e.report.cpu.usage) + '%' }" /></div>
                        <span class="mr-val">{{ e.report.cpu.usage.toFixed(1) }}%</span>
                    </div>
                    <div class="meter-row">
                        <span class="mr-label">RAM</span>
                        <div class="meter"><i :class="meterClass(percent(e.report.ram))" :style="{ width: percent(e.report.ram) + '%' }" /></div>
                        <span class="mr-val">{{ formatBytes(e.report.ram.used) }} / {{ formatBytes(e.report.ram.total) }}</span>
                    </div>
                    <div class="meter-row">
                        <span class="mr-label">Disk</span>
                        <div class="meter"><i :class="meterClass(percent(e.report.disk))" :style="{ width: percent(e.report.disk) + '%' }" /></div>
                        <span class="mr-val">{{ formatBytes(e.report.disk.used) }} / {{ formatBytes(e.report.disk.total) }}</span>
                    </div>
                </template>

                <template v-else-if="e.online">
                    <p class="no-report">Online, waiting for the first status report…</p>
                </template>

                <template v-else>
                    <p class="no-report">{{ e.report && e.report.message ? e.report.message : 'Currently offline' }}</p>
                </template>

                <footer class="row-3">
                    <div class="spec" v-if="e.info && e.info.cpu">
                        <span>{{ e.info.os ? e.info.os.os || '—' : '—' }}</span>
                        <span>·</span>
                        <span>{{ e.info.cpu.cpu_cores ? e.info.cpu.cpu_cores + ' cores' : '' }}</span>
                        <span>·</span>
                        <span class="mono">{{ e.info.ipv4 || (e.info.ipv6 ? 'IPv6' : '—') }}</span>
                    </div>
                    <div class="notify">
                        <Toggle
                            :model-value="e.notify"
                            label="Status notify"
                            :disabled="saving.has(e.uuid)"
                            @update:model-value="toggleNotify(e)"
                        />
                    </div>
                </footer>
            </article>
        </div>
    </section>
</template>

<style scoped>
.server-card { transition: opacity 0.25s ease; }
.server-card.dim .sv-icon,
.server-card.dim .sv-name { opacity: 0.75; }

.sv-icon {
    width: 40px;
    height: 40px;
    flex: none;
    border-radius: 11px;
    display: grid;
    place-items: center;
    background: var(--accent-soft);
    color: var(--accent);
    font-size: 17px;
}

.server-card.dim .sv-icon {
    background: var(--surface-3);
    color: var(--text-3);
}

.sv-main {
    flex: 1;
    min-width: 0;
}

.sv-uuid {
    font-family: var(--font-mono);
    font-size: 11.5px;
    color: var(--text-3);
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 0;
}

.sv-uuid:hover { color: var(--accent); }

.meter-row {
    display: grid;
    grid-template-columns: 44px 1fr auto;
    align-items: center;
    gap: 10px;
}

.mr-label { font-size: 12px; color: var(--text-3); font-weight: 600; }

.mr-val {
    font-size: 12px;
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
    min-width: 90px;
    text-align: right;
}

.no-report {
    font-size: 13px;
    color: var(--text-3);
    padding: 4px 0;
}

.row-3 {
    border-top: 1px solid var(--line);
    padding-top: 13px;
}

.spec {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--text-3);
    min-width: 0;
    flex-wrap: wrap;
}

.notify { display: flex; align-items: center; }

.refresh-hint {
    font-size: 12px;
    color: var(--text-3);
    display: inline-flex;
    align-items: center;
    gap: 6px;
}
</style>
