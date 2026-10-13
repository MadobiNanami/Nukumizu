<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { serverApi, settingsApi } from '../api/index.js';
import { toast } from '../utils/toast.js';
import { formatBytes, shortUUID } from '../utils/fmt.js';
import Toggle from '../components/Toggle.vue';
import Modal from '../components/Modal.vue';

function percent(m) {
    return m && m.total ? Math.min(100, Math.max(0, Math.round((m.used / m.total) * 100))) : 0;
}

const query = ref('');
const loading = ref(true);
const failed = ref(false);
const entries = ref([]);
let timer = null;

// The per-node options dialog. Its form is a copy of the node's settings taken
// when the dialog opens, so the 15s card refresh behind it cannot disturb an
// open editor, and closing without saving leaves the node untouched.
const editing = reactive({
    open: false,
    uuid: '',
    name: '',
    notify: true,
    flap: '',
    saving: false
});

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
        const statusData = (status && status.data) || {};
        const infoData = (info && info.data) || {};
        const nodeConf = (nodes && nodes.data && nodes.data.config) || {};
        const list = [];
        for (const key of Object.keys(statusData)) {
            const s = statusData[key] || {};
            const infoEntry = infoData[key] || {};
            const conf = nodeConf[key] || {};
            list.push({
                uuid: s.uuid || key,
                name: s.name || shortUUID(s.uuid || key),
                online: !!s.online,
                report: s.report || null,
                info: infoEntry.info || null,
                notify: conf.enableStatusNotify !== false,
                flapTolerant: Number(conf.flap_tolerant) > 0 ? Number(conf.flap_tolerant) : 0
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

function openOptions(entry) {
    editing.uuid = entry.uuid;
    editing.name = entry.name;
    editing.notify = entry.notify;
    editing.flap = entry.flapTolerant ? String(entry.flapTolerant) : '';
    editing.open = true;
}

// saveOptions writes both options in one patch. A flap window that is not a
// positive number disables inhibition, which is sent as a JSON null so the key
// is removed rather than stored as an explicit 0 — the file then keeps matching
// its documented "absent means off" default.
async function saveOptions() {
    const typed = Number(editing.flap);
    const seconds = Number.isFinite(typed) && typed > 0 ? typed : 0;
    const uuid = editing.uuid;

    editing.saving = true;
    try {
        const patch = {};
        patch[uuid] = {
            enableStatusNotify: editing.notify,
            flap_tolerant: seconds > 0 ? seconds : null
        };
        await settingsApi.set('bot_node_config', patch);
        const entry = entries.value.find((e) => e.uuid === uuid);
        if (entry) {
            entry.notify = editing.notify;
            entry.flapTolerant = seconds;
        }
        toast.success(`${editing.name}: options saved`);
        editing.open = false;
    } catch (e) {
        toast.error('Save failed: ' + e.message);
    } finally {
        editing.saving = false;
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
                    <div class="opts">
                        <span v-if="!e.notify" class="chip" title="Status-change notifications are off for this node">
                            <i class="fas fa-bell-slash" /> Notify off
                        </span>
                        <span v-if="e.flapTolerant" class="chip" title="Flap inhibition window">
                            <i class="fas fa-shield-halved" /> Flap {{ e.flapTolerant }}s
                        </span>
                        <button class="icon-btn" title="Node options" @click="openOptions(e)">
                            <i class="fas fa-pen" />
                        </button>
                    </div>
                </footer>
            </article>
        </div>

        <Modal :open="editing.open" title="Node options" @close="editing.open = false">
            <div class="node-head">
                <p class="node-name">{{ editing.name }}</p>
                <p class="node-uuid mono">{{ editing.uuid }}</p>
            </div>

            <div class="field">
                <label>Status notify</label>
                <Toggle v-model="editing.notify" label="Report online / offline changes" />
                <p class="help">With this off, the node's status changes are not delivered to any pipes.</p>
            </div>

            <div class="field">
                <label>Flap inhibition</label>
                <div class="flap-row">
                    <input
                        v-model="editing.flap"
                        class="input flap-input"
                        type="number"
                        min="0"
                        max="86400"
                        step="1"
                        inputmode="numeric"
                        placeholder="0"
                        @keydown.enter="saveOptions"
                    />
                    <span class="flap-unit">seconds</span>
                </div>
                <p class="help">How long the node may drop offline and come back without a notification being sent. An outage lasting longer than this is still reported. Leave empty or 0 to report every change right away.</p>
            </div>

            <template #foot>
                <button class="btn btn-ghost" @click="editing.open = false">Cancel</button>
                <button class="btn btn-primary" :disabled="editing.saving" @click="saveOptions">
                    <span v-if="editing.saving" class="spinner" style="width:12px;height:12px" />
                    <i v-else class="fas fa-check" /> Save
                </button>
            </template>
        </Modal>
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

.opts {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
    /* Stay right-aligned even when the spec block is absent. */
    margin-left: auto;
}

.opts .chip { white-space: nowrap; }

/* --- Node options dialog ------------------------------------------------ */

.node-head {
    padding-bottom: 14px;
    margin-bottom: 16px;
    border-bottom: 1px solid var(--line);
}

.node-name {
    font-size: 15px;
    font-weight: 600;
    color: var(--text);
}

.node-uuid {
    font-size: 11.5px;
    color: var(--text-3);
    margin-top: 3px;
    overflow-wrap: anywhere;
}

/* The modal body has no grid to space stacked fields. */
.field + .field { margin-top: 20px; }

.flap-row {
    display: flex;
    align-items: center;
    gap: 9px;
}

.flap-input {
    width: 110px;
    text-align: center;
    font-variant-numeric: tabular-nums;
    /* Match the plain hand-rolled field look: the rest of the console renders
       numbers as ordinary inputs, so the spinner controls would stand out. */
    -moz-appearance: textfield;
    appearance: textfield;
}

.flap-input::-webkit-outer-spin-button,
.flap-input::-webkit-inner-spin-button {
    -webkit-appearance: none;
    margin: 0;
}

.flap-unit {
    font-size: 12.5px;
    color: var(--text-3);
}

.refresh-hint {
    font-size: 12px;
    color: var(--text-3);
    display: inline-flex;
    align-items: center;
    gap: 6px;
}
</style>
