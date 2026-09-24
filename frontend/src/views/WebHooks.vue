<script setup>
import { computed, onMounted, reactive, ref } from 'vue';
import { settingsApi, webhookApi } from '../api/index.js';
import { toast } from '../utils/toast.js';
import ConfigSection from '../components/ConfigSection.vue';
import Toggle from '../components/Toggle.vue';
import Modal from '../components/Modal.vue';

// The notification channels an endpoint may relay to. These mirror the
// controller names in config.json (controllerMethod.*), which is exactly what
// the backend matches notifyPipes against.
const CHANNELS = [
    { key: 'qq(napcat)', label: 'QQ' },
    { key: 'telegram', label: 'Telegram' },
    { key: 'email', label: 'Email' },
    { key: 'ntfy', label: 'ntfy' },
    { key: 'webhook', label: 'WebHook' }
];

// The listener half of the incoming webhook API. The endpoints it serves are
// managed through /api/webhook/* instead of the settings API, because they are
// a keyed collection rather than a fixed set of fields. The outgoing WebHook
// notification channel stays on the Settings page with the other channels.
const apiSection = {
    id: 'webhookApi',
    title: 'Incoming API',
    hint: 'Listener external applications post to. The address and port are read at startup — changing them needs a restart.',
    root: ['webhook'],
    fields: [
        { key: 'enabled', type: 'bool', label: 'Enabled', help: 'Disabled means no listener is started at all.' },
        { key: 'listenAddr', type: 'text', label: 'Listen address' },
        { key: 'listenPort', type: 'text', label: 'Listen port' }
    ]
};

const loading = ref(true);
const failed = ref(false);
const cfg = ref({});
const endpoints = ref([]);
const saving = reactive(new Set());
const revealed = reactive(new Set());

const editor = reactive({ open: false, mode: 'add', name: '', token: '', pipes: [] });
const removing = reactive({ open: false, name: '' });

const channelLabel = (key) => (CHANNELS.find((c) => c.key === key) || { label: key }).label;

// Endpoint URLs are on the webhook listener, not the one serving this console,
// so the host is taken from the browser and the port from the configuration.
const listenPort = computed(() => (cfg.value.webhook && cfg.value.webhook.listenPort) || '8081');
const endpointURL = (name) => `http://${window.location.hostname}:${listenPort.value}/api/webhook/post/${name}`;

function normalizeEndpoint(name, e) {
    return {
        name,
        enabled: e.enabled === true,
        token: typeof e.token === 'string' ? e.token : '',
        notifyPipes: Array.isArray(e.notifyPipes) ? e.notifyPipes : []
    };
}

async function load() {
    try {
        const [settings, list] = await Promise.all([settingsApi.get('global'), webhookApi.list()]);
        cfg.value = (settings && settings.data && settings.data.config) || {};
        const map = (list && list.data && list.data.endpoints) || {};
        endpoints.value = Object.entries(map)
            .map(([name, e]) => normalizeEndpoint(name, e))
            .sort((a, b) => a.name.localeCompare(b.name));
        failed.value = false;
    } catch (e) {
        failed.value = true;
        if (e.status) toast.error('Failed to load WebHooks: ' + e.message);
    } finally {
        loading.value = false;
    }
}

async function saveEndpoint(name, fields, okMsg) {
    saving.add(name);
    try {
        await webhookApi.modify({ name, ...fields });
        if (okMsg) toast.success(okMsg);
        return true;
    } catch (e) {
        toast.error('Save failed: ' + e.message);
        return false;
    } finally {
        saving.delete(name);
    }
}

async function toggleEnabled(row, value) {
    const prev = row.enabled;
    row.enabled = value;
    if (!(await saveEndpoint(row.name, { enabled: value }))) {
        row.enabled = prev;
    }
}

function openAdd() {
    editor.mode = 'add';
    editor.name = '';
    editor.token = generateToken();
    editor.pipes = [];
    editor.open = true;
}

function openEdit(row) {
    editor.mode = 'edit';
    editor.name = row.name;
    editor.token = row.token;
    editor.pipes = [...row.notifyPipes];
    editor.open = true;
}

function togglePipe(key) {
    const at = editor.pipes.indexOf(key);
    if (at === -1) editor.pipes.push(key);
    else editor.pipes.splice(at, 1);
}

// A 30-character hex token, so a new endpoint never starts out with a guessable
// shared secret.
function generateToken() {
    const bytes = new Uint8Array(15);
    crypto.getRandomValues(bytes);
    return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

async function confirmEditor() {
    const name = editor.name.trim();
    if (!name) return toast.warn('Enter a name');
    if (name.includes('/')) return toast.warn('The name cannot contain "/"');
    // The backend refuses requests to an endpoint that has no token, so an
    // empty one would only ever answer 500.
    if (!editor.token) return toast.warn('Enter or generate a token');

    const body = { name, token: editor.token, notifyPipes: [...editor.pipes] };
    try {
        if (editor.mode === 'add') {
            // A new endpoint starts enabled; the switch in the list turns it off.
            await webhookApi.add({ ...body, enabled: true });
            toast.success('Endpoint added');
        } else {
            // enabled is deliberately left out: the list switch owns it, and a
            // value read when the editor opened could undo a toggle made since.
            await webhookApi.modify(body);
            toast.success('Endpoint updated');
        }
        editor.open = false;
        await load();
    } catch (e) {
        toast.error('Save failed: ' + e.message);
    }
}

function askRemove(row) {
    removing.name = row.name;
    removing.open = true;
}

async function confirmRemove() {
    try {
        await webhookApi.remove(removing.name);
        toast.success('Endpoint removed');
        removing.open = false;
        await load();
    } catch (e) {
        toast.error('Remove failed: ' + e.message);
    }
}

function copy(value, okMsg) {
    if (!navigator.clipboard) return toast.warn('Clipboard unavailable — copy it manually');
    navigator.clipboard.writeText(value).then(() => toast.success(okMsg), () => {});
}

onMounted(load);
</script>

<template>
    <section class="page">
        <header class="page-head">
            <div>
                <p class="eyebrow">Integrations</p>
                <h1>WebHooks</h1>
                <p class="lead">
                    Let external applications push alerts through this program and relay them to the notification channels.
                    Where this program posts its own alerts is configured on the Settings page.
                </p>
            </div>
            <div class="head-actions">
                <button class="btn btn-ghost" @click="load">
                    <i class="fas fa-rotate" :class="{ 'fa-spin': loading }" /> Reload
                </button>
            </div>
        </header>

        <div v-if="failed" class="card empty">
            <i class="fas fa-plug-circle-xmark e-icon" />
            <p>Failed to load WebHook settings</p>
            <button class="btn btn-primary" @click="load">Retry</button>
        </div>

        <div v-else-if="loading" class="card empty"><span class="spinner" /></div>

        <template v-else>
            <ConfigSection :section="apiSection" :config="cfg" @saved="load" />

            <div class="card">
                <div class="card-head">
                    <div>
                        <h3>Endpoints</h3>
                        <p class="hint">One endpoint per external application. Each carries its own token and relays to its own channels.</p>
                    </div>
                    <button class="btn btn-primary btn-sm" @click="openAdd">
                        <i class="fas fa-plus" /> Add endpoint
                    </button>
                </div>

                <div class="card-body">
                    <div v-if="endpoints.length === 0" class="empty" style="padding:28px">
                        No endpoints yet — add one to get an URL to post to
                    </div>
                    <div v-else class="opt-list">
                        <div v-for="e in endpoints" :key="e.name" class="ep-row">
                            <div class="ep-main">
                                <div class="ep-title">
                                    <span class="id mono">{{ e.name }}</span>
                                    <span class="badge" :class="e.enabled ? 'badge-online' : 'badge-offline'">
                                        <span class="dot" />{{ e.enabled ? 'Enabled' : 'Disabled' }}
                                    </span>
                                </div>
                                <div class="ep-line">
                                    <span class="method mono">POST</span>
                                    <span class="url mono">{{ endpointURL(e.name) }}</span>
                                    <button class="icon-btn" title="Copy URL" @click="copy(endpointURL(e.name), 'Endpoint URL copied')">
                                        <i class="fas fa-link" />
                                    </button>
                                </div>
                                <div class="ep-line">
                                    <span class="lbl">Token</span>
                                    <span class="url mono">{{ revealed.has(e.name) ? e.token : '••••••••••••' }}</span>
                                    <button
                                        class="icon-btn"
                                        :title="revealed.has(e.name) ? 'Hide token' : 'Show token'"
                                        @click="revealed.has(e.name) ? revealed.delete(e.name) : revealed.add(e.name)"
                                    >
                                        <i class="fas" :class="revealed.has(e.name) ? 'fa-eye-slash' : 'fa-eye'" />
                                    </button>
                                    <button class="icon-btn" title="Copy token" @click="copy(e.token, 'Token copied')">
                                        <i class="fas fa-copy" />
                                    </button>
                                </div>
                                <div class="ep-channels">
                                    <span v-if="e.notifyPipes.length === 0" class="badge badge-danger">
                                        <span class="dot" />No channels
                                    </span>
                                    <span v-for="p in e.notifyPipes" :key="p" class="badge badge-accent">{{ channelLabel(p) }}</span>
                                </div>
                            </div>

                            <div class="ep-actions">
                                <Toggle
                                    :model-value="e.enabled"
                                    :disabled="saving.has(e.name)"
                                    @update:model-value="toggleEnabled(e, $event)"
                                />
                                <button class="icon-btn" title="Edit" @click="openEdit(e)"><i class="fas fa-pen" /></button>
                                <button class="icon-btn danger" title="Remove" @click="askRemove(e)"><i class="fas fa-trash" /></button>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </template>

        <Modal
            :open="editor.open"
            :title="editor.mode === 'add' ? 'Add endpoint' : 'Edit endpoint'"
            @close="editor.open = false"
        >
            <div class="form-grid">
                <div class="field span-2">
                    <label>Name</label>
                    <input
                        v-model="editor.name"
                        class="input mono"
                        :disabled="editor.mode === 'edit'"
                        placeholder="example"
                        spellcheck="false"
                        @keydown.enter="confirmEditor"
                    />
                    <p class="help">
                        Posted to <span class="mono">/api/webhook/post/&lt;name&gt;</span>. The name cannot be changed afterwards.
                    </p>
                    <p v-if="editor.mode === 'add'" class="help">
                        The endpoint starts enabled — use the switch in the list to disable it.
                    </p>
                </div>

                <div class="field span-2">
                    <label>Token</label>
                    <div class="token-row">
                        <input v-model="editor.token" class="input mono" spellcheck="false" autocomplete="off" />
                        <button class="btn btn-ghost btn-sm" @click="editor.token = generateToken()">
                            <i class="fas fa-dice" /> Generate
                        </button>
                    </div>
                    <p class="help">The caller sends this in the request body. An endpoint without a token rejects every request.</p>
                </div>

                <div class="field span-2">
                    <label>Relay to</label>
                    <div class="pipe-picker">
                        <button
                            v-for="c in CHANNELS"
                            :key="c.key"
                            type="button"
                            class="pipe"
                            :class="{ on: editor.pipes.includes(c.key) }"
                            @click="togglePipe(c.key)"
                        >
                            <i class="fas" :class="editor.pipes.includes(c.key) ? 'fa-square-check' : 'fa-square'" />
                            {{ c.label }}
                        </button>
                    </div>
                    <p class="help">
                        The alert is relayed to every channel selected here; a channel that is disabled is skipped.
                        The WebHook channel's own destination is configured on the Settings page.
                    </p>
                </div>
            </div>

            <template #foot>
                <button class="btn btn-ghost" @click="editor.open = false">Cancel</button>
                <button class="btn btn-primary" @click="confirmEditor">
                    {{ editor.mode === 'add' ? 'Add' : 'Save' }}
                </button>
            </template>
        </Modal>

        <Modal :open="removing.open" title="Remove endpoint" @close="removing.open = false">
            <p style="line-height:1.6">
                Remove <strong class="mono">{{ removing.name }}</strong>? Requests to its URL will stop being accepted.
            </p>
            <template #foot>
                <button class="btn btn-ghost" @click="removing.open = false">Cancel</button>
                <button class="btn btn-danger" @click="confirmRemove">Remove</button>
            </template>
        </Modal>
    </section>
</template>

<style scoped>
.ep-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 18px;
    padding: 14px;
    border-bottom: 1px solid var(--line);
}

.ep-row:last-child {
    border-bottom: 0;
}

.ep-main {
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 0;
}

.ep-title {
    display: flex;
    align-items: center;
    gap: 10px;
}

.ep-title .id {
    font-family: var(--font-mono);
    font-size: 14px;
    font-weight: 600;
}

.ep-line {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
}

.ep-line .lbl {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-3);
    font-weight: 600;
}

.ep-line .url {
    font-family: var(--font-mono);
    font-size: 12.5px;
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.method {
    font-size: 10.5px;
    font-weight: 700;
    letter-spacing: 0.06em;
    padding: 1px 6px;
    border-radius: var(--r-s);
    background: var(--accent-soft);
    color: var(--accent);
}

.ep-channels {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
    margin-top: 2px;
}

.ep-actions {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-shrink: 0;
}

.token-row {
    display: flex;
    align-items: center;
    gap: 8px;
}

.token-row .input {
    flex: 1;
    min-width: 0;
}

.pipe-picker {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
}

.pipe {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    padding: 6px 11px;
    border: 1px solid var(--line-strong);
    border-radius: var(--r-pill);
    background: var(--surface-2);
    color: var(--text-3);
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition: border-color 0.14s ease, color 0.14s ease, background 0.14s ease;
}

.pipe:hover {
    color: var(--text);
}

.pipe.on {
    border-color: var(--accent);
    background: var(--accent-soft);
    color: var(--accent);
    font-weight: 600;
}
</style>
