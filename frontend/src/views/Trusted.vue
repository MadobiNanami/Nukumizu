<script setup>
import { computed, onMounted, reactive, ref } from 'vue';
import { settingsApi } from '../api/index.js';
import { toast } from '../utils/toast.js';
import Toggle from '../components/Toggle.vue';
import Modal from '../components/Modal.vue';

const CHANNELS = [
    { key: 'qq(napcat)', label: 'QQ (NapCat)', hint: 'Members are identified by QQ number / group number' },
    { key: 'telegram', label: 'Telegram', hint: 'Members are identified by @username or numeric ID / group ID' }
];

const KINDS = [
    { key: 'admins', label: 'Admins' },
    { key: 'trustedGroups', label: 'Trusted groups' }
];

const OPTIONS = [
    { key: 'event_status_notify', label: 'Status notify' },
    { key: 'event_bot_started', label: 'Startup notify' },
    { key: 'event_reply', label: 'Command replies' }
];

const loading = ref(true);
const failed = ref(false);
const rows = ref([]);
const saving = reactive(new Set());

const addModal = reactive({ open: false, ch: CHANNELS[0].key, kind: 'admins', id: '' });
const removing = reactive({ open: false, ch: '', kind: '', id: '' });

const rowsOf = (ch, kind) => rows.value
    .filter((r) => r.ch === ch && r.kind === kind)
    .sort((a, b) => a.id.localeCompare(b.id));

const summary = computed(() => {
    const out = {};
    for (const c of CHANNELS) {
        out[c.key] = { admins: rowsOf(c.key, 'admins').length, trustedGroups: rowsOf(c.key, 'trustedGroups').length };
    }
    return out;
});

const defaultOpts = () => ({
    event_status_notify: true,
    event_bot_started: true,
    event_reply: true
});

async function load() {
    try {
        const res = await settingsApi.get('bot_user_config');
        const cfg = (res && res.config) || {};
        const flat = [];
        for (const c of CHANNELS) {
            const chData = cfg[c.key] || {};
            for (const k of KINDS) {
                const map = chData[k.key] || {};
                for (const [id, opts] of Object.entries(map)) {
                    flat.push({
                        ch: c.key,
                        kind: k.key,
                        id,
                        opts: {
                            event_status_notify: opts.event_status_notify !== false,
                            event_bot_started: opts.event_bot_started !== false,
                            event_reply: opts.event_reply !== false
                        }
                    });
                }
            }
        }
        rows.value = flat;
        failed.value = false;
    } catch (e) {
        failed.value = true;
        if (e.status) toast.error('Failed to load: ' + e.message);
    } finally {
        loading.value = false;
    }
}

function patchRow(row) {
    const p = {};
    p[row.ch] = { [row.kind]: { [row.id]: { ...row.opts } } };
    return p;
}

async function applyPatch(patch, okMsg, rollbackFn) {
    try {
        await settingsApi.set('bot_user_config', patch);
        if (okMsg) toast.success(okMsg);
        return true;
    } catch (e) {
        if (rollbackFn) rollbackFn();
        toast.error('Save failed: ' + e.message);
        return false;
    }
}

function toggleOpt(row, key, value) {
    const prev = row.opts[key];
    row.opts[key] = value;
    applyPatch(patchRow(row), undefined, () => { row.opts[key] = prev; });
}

function openAdd(ch, kind) {
    addModal.ch = ch;
    addModal.kind = kind;
    addModal.id = '';
    addModal.open = true;
}

async function confirmAdd() {
    const id = addModal.id.trim();
    if (!id) return toast.warn('Enter a member ID');
    const exists = rows.value.some((r) => r.ch === addModal.ch && r.kind === addModal.kind && r.id === id);
    if (exists) {
        toast.warn('That member already exists');
        return;
    }
    const row = { ch: addModal.ch, kind: addModal.kind, id, opts: defaultOpts() };
    if (await applyPatch(patchRow(row), 'Member added')) {
        rows.value.push(row);
        addModal.open = false;
    }
}

function askRemove(row) {
    removing.ch = row.ch;
    removing.kind = row.kind;
    removing.id = row.id;
    removing.open = true;
}

async function confirmRemove() {
    const { ch, kind, id } = removing;
    const p = {};
    p[ch] = { [kind]: { [id]: null } };
    if (await applyPatch(p, 'Member removed')) {
        rows.value = rows.value.filter((r) => !(r.ch === ch && r.kind === kind && r.id === id));
        removing.open = false;
    }
}

onMounted(load);
</script>

<template>
    <section class="page">
        <header class="page-head">
            <div>
                <p class="eyebrow">Bot trust</p>
                <h1>Trust management</h1>
                <p class="lead">Which admins and groups each bot channel answers to, plus per-member notification and reply switches.</p>
            </div>
            <div class="head-actions">
                <button class="btn btn-ghost" @click="load">
                    <i class="fas fa-rotate" :class="{ 'fa-spin': loading }" /> Reload
                </button>
            </div>
        </header>

        <div v-if="failed" class="card empty">
            <i class="fas fa-plug-circle-xmark e-icon" />
            <p>Failed to load, please check the backend connection</p>
            <button class="btn btn-primary" @click="load">Retry</button>
        </div>

        <div v-for="c in CHANNELS" :key="c.key" class="card">
            <div class="card-head">
                <div>
                    <h3>{{ c.label }}</h3>
                    <p class="hint">{{ c.hint }}</p>
                </div>
                <span class="chip">
                    {{ summary[c.key].admins }} admins · {{ summary[c.key].trustedGroups }} groups
                </span>
            </div>

            <div class="card-body">
                <div v-for="k in KINDS" :key="k.key" class="kind-block">
                    <div class="kind-head">
                        <span class="kind-title">{{ k.label }}</span>
                        <button class="btn btn-ghost btn-sm" @click="openAdd(c.key, k.key)">
                            <i class="fas fa-plus" /> Add
                        </button>
                    </div>

                    <div v-if="loading" class="empty" style="padding:24px"><span class="spinner" /></div>
                    <div v-else-if="rowsOf(c.key, k.key).length === 0" class="empty" style="padding:22px">
                        No {{ k.label.toLowerCase() }} yet
                    </div>
                    <div v-else class="opt-list">
                        <div v-for="r in rowsOf(c.key, k.key)" :key="r.id" class="opt-row">
                            <div class="who">
                                <span class="id mono">{{ r.id }}</span>
                            </div>
                            <div class="toggles">
                                <div v-for="o in OPTIONS" :key="o.key" class="oc">
                                    <span class="oc-label">{{ o.label }}</span>
                                    <Toggle
                                        :model-value="r.opts[o.key]"
                                        @update:model-value="toggleOpt(r, o.key, $event)"
                                    />
                                </div>
                                <button class="icon-btn danger" title="Remove" @click="askRemove(r)">
                                    <i class="fas fa-trash" />
                                </button>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <Modal :open="addModal.open" title="Add member" @close="addModal.open = false">
            <div class="form-grid">
                <div class="field">
                    <label>Channel</label>
                    <select v-model="addModal.ch" class="select">
                        <option v-for="c in CHANNELS" :key="c.key" :value="c.key">{{ c.label }}</option>
                    </select>
                </div>
                <div class="field">
                    <label>Type</label>
                    <select v-model="addModal.kind" class="select">
                        <option v-for="k in KINDS" :key="k.key" :value="k.key">{{ k.label }}</option>
                    </select>
                </div>
                <div class="field span-2">
                    <label>Member ID</label>
                    <input
                        v-model="addModal.id"
                        class="input mono"
                        placeholder="QQ / group number, or Telegram @username / numeric ID"
                        @keydown.enter="confirmAdd"
                    />
                    <p class="help">New members start with every notification enabled; adjust them in the list.</p>
                </div>
            </div>
            <template #foot>
                <button class="btn btn-ghost" @click="addModal.open = false">Cancel</button>
                <button class="btn btn-primary" @click="confirmAdd">Add</button>
            </template>
        </Modal>

        <Modal :open="removing.open" title="Remove member" @close="removing.open = false">
            <p style="line-height:1.6">
                Remove <strong class="mono">{{ removing.id }}</strong> from
                <strong>{{ KINDS.find((k) => k.key === removing.kind)?.label }}</strong>?
            </p>
            <template #foot>
                <button class="btn btn-ghost" @click="removing.open = false">Cancel</button>
                <button class="btn btn-danger" @click="confirmRemove">Remove</button>
            </template>
        </Modal>
    </section>
</template>

<style scoped>
.kind-block + .kind-block { margin-top: 20px; }

.kind-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;
}

.kind-title {
    font-size: 13px;
    font-weight: 600;
    letter-spacing: 0.02em;
    color: var(--text-2);
}

.toggles {
    display: flex;
    align-items: center;
    gap: 18px;
    flex-wrap: wrap;
}

.oc {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 3px;
}

.oc-label {
    font-size: 11px;
    color: var(--text-3);
    font-weight: 500;
    letter-spacing: 0.02em;
}
</style>
