<script setup>
import { onMounted, reactive, ref } from 'vue';
import { settingsApi } from '../api/index.js';
import { debugMode } from '../utils/runtime.js';
import { toast } from '../utils/toast.js';
import Toggle from '../components/Toggle.vue';
import TagsEditor from '../components/TagsEditor.vue';
import HeadersEditor from '../components/HeadersEditor.vue';

const sections = [
    {
        id: 'system',
        title: 'System',
        hint: 'HTTP listener and global runtime switches.',
        root: ['system'],
        fields: [
            { key: 'debugMode', type: 'bool', label: 'Debug mode', help: 'Skipped X-Timestamp checks and verbose debug logging.' },
            { key: 'listenAddr', type: 'text', label: 'Listen address' },
            { key: 'listenPort', type: 'text', label: 'Listen port' },
            { key: 'networkProxy', type: 'text', label: 'Network proxy', placeholder: 'http://host:port' }
        ]
    },
    {
        id: 'debug',
        title: 'Debug logging',
        hint: 'Per-module verbosity for the bot pipes.',
        root: ['debug'],
        fields: [
            { key: 'showNapcatMsg', type: 'bool', label: 'NapCat messages' },
            { key: 'showNapcatAction', type: 'bool', label: 'NapCat actions' },
            { key: 'showTelegramMsg', type: 'bool', label: 'Telegram messages' },
            { key: 'showTriggerCmdEcho', type: 'bool', label: 'Trigger command echo' },
            { key: 'showKomariTaskEcho', type: 'bool', label: 'Komari task echo' },
            { key: 'napcatIgnoreSelfMsg', type: 'bool', label: 'Ignore NapCat self messages' }
        ]
    },
    {
        id: 'komari',
        title: 'Komari dashboard',
        hint: 'Connection the monitor reads node data from. Takes effect on restart.',
        root: ['komari'],
        fields: [
            { key: 'dashboardURL', type: 'text', label: 'Dashboard URL' },
            { key: 'account.username', lp: ['account', 'username'], type: 'text', label: 'Account' },
            { key: 'account.password', lp: ['account', 'password'], type: 'password', label: 'Password' }
        ]
    },
    {
        id: 'controllerMessage',
        title: 'Message templates',
        hint: 'Templates rendered for bot pushes. Placeholders like {{ serverName }} stay as-is.',
        root: ['controllerMessage'],
        fields: [
            { key: 'BOT_STARTED', type: 'textarea', label: 'BOT_STARTED' },
            { key: 'BOT_HELP', type: 'textarea', label: 'BOT_HELP' },
            { key: 'TG_BOT_START', type: 'textarea', label: 'TG_BOT_START' },
            { key: 'SERVER_STATUS_CHANGED', type: 'textarea', label: 'SERVER_STATUS_CHANGED' },
            { key: 'SERVER_LIST', type: 'textarea', label: 'SERVER_LIST' },
            { key: 'SERVER_EXECUTE_RESULT', type: 'textarea', label: 'SERVER_EXECUTE_RESULT' }
        ]
    },
    {
        id: 'qq',
        title: 'QQ controller (NapCat)',
        root: ['controllerMethod', 'qq(napcat)'],
        fields: [
            { key: 'enabled', type: 'bool', label: 'Enabled' },
            { key: 'networkUseProxy', type: 'bool', label: 'Use network proxy' },
            { key: 'napcatAddr', type: 'text', label: 'NapCat address' },
            { key: 'napcatPort', type: 'text', label: 'NapCat port' },
            { key: 'napcatToken', type: 'password', label: 'NapCat token' },
            { key: 'botQQID', type: 'number', label: 'Bot QQ ID' },
            { key: 'listenMethod', type: 'select', label: 'Listen method', options: ['global', 'at'] }
        ]
    },
    {
        id: 'telegram',
        title: 'Telegram controller',
        root: ['controllerMethod', 'telegram'],
        fields: [
            { key: 'enabled', type: 'bool', label: 'Enabled' },
            { key: 'networkUseProxy', type: 'bool', label: 'Use network proxy' },
            { key: 'botToken', type: 'password', label: 'Bot token' },
            { key: 'listenMethod', type: 'select', label: 'Listen method', options: ['global', 'at'] }
        ]
    },
    {
        id: 'email',
        title: 'Email notifications',
        root: ['controllerMethod', 'email'],
        fields: [
            { key: 'enabled', type: 'bool', label: 'Enabled' },
            { key: 'networkUseProxy', type: 'bool', label: 'Use network proxy' },
            { key: 'smtpHost', type: 'text', label: 'SMTP host' },
            { key: 'smtpPort', type: 'number', label: 'SMTP port' },
            { key: 'username', type: 'text', label: 'Username' },
            { key: 'password', type: 'password', label: 'Password' },
            { key: 'from', type: 'text', label: 'From address' },
            { key: 'to', type: 'tags', label: 'Recipients', placeholder: 'Type an address and press Enter' },
            { key: 'useTLS', type: 'bool', label: 'Use TLS' }
        ]
    },
    {
        id: 'ntfy',
        title: 'Ntfy notifications',
        root: ['controllerMethod', 'ntfy'],
        fields: [
            { key: 'enabled', type: 'bool', label: 'Enabled' },
            { key: 'networkUseProxy', type: 'bool', label: 'Use network proxy' },
            { key: 'server', type: 'text', label: 'Server' },
            { key: 'topic', type: 'text', label: 'Topic' },
            { key: 'token', type: 'password', label: 'Token' },
            { key: 'priority', type: 'select', label: 'Priority', options: ['min', 'low', 'default', 'high', 'urgent', 'max'] }
        ]
    },
    {
        id: 'webhook',
        title: 'Webhook notifications',
        root: ['controllerMethod', 'webhook'],
        fields: [
            { key: 'enabled', type: 'bool', label: 'Enabled' },
            { key: 'networkUseProxy', type: 'bool', label: 'Use network proxy' },
            { key: 'url', type: 'text', label: 'URL' },
            { key: 'method', type: 'select', label: 'Method', options: ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'] },
            { key: 'headers', type: 'headers', label: 'Headers' },
            { key: 'template', type: 'textarea', label: 'Payload template' }
        ]
    },
    {
        id: 'paths',
        title: 'Storage paths',
        hint: 'Where runtime data lives. Takes effect on restart.',
        root: [],
        fields: [
            { key: 'dataPath', type: 'text', label: 'Data path' },
            { key: 'dbPath', type: 'text', label: 'Database path' }
        ]
    }
];

const loading = ref(true);
const failed = ref(false);
const vals = reactive({});
const saving = reactive(new Set());

function fieldPath(f) {
    return f.lp || [f.key];
}

function getVal(obj, path, fb) {
    let cur = obj;
    for (const k of path) {
        if (cur === null || cur === undefined || typeof cur !== 'object') return fb;
        cur = cur[k];
    }
    return cur === undefined || cur === null ? fb : cur;
}

// hasVal reports whether a path actually resolves in the loaded config. A
// missing key and a key whose value equals the fallback are indistinguishable
// from getVal's return value alone, so misses are detected separately.
function hasVal(obj, path) {
    let cur = obj;
    for (const k of path) {
        if (cur === null || cur === undefined || typeof cur !== 'object') return false;
        cur = cur[k];
    }
    return cur !== undefined && cur !== null;
}

function defaults(f) {
    switch (f.type) {
        case 'bool': return false;
        case 'number': return 0;
        case 'tags': return [];
        case 'headers': return {};
        default: return '';
    }
}

async function load() {
    try {
        const res = await settingsApi.get('global');
        const cfg = (res && res.data && res.data.config) || {};
        for (const s of sections) {
            const obj = {};
            for (const f of s.fields) {
                // Mirror wrapRoot() on save: prepend the section's root path so
                // the value is read from the same place it is written to.
                const path = [...s.root, ...fieldPath(f)];
                obj[f.key] = getVal(cfg, path, defaults(f));
                if (debugMode.value) {
                    console.log(`[Settings] Loaded ${s.id}.${f.key}:`, obj[f.key]);
                    if (!hasVal(cfg, path)) {
                        console.warn(`[Settings] ${s.id}.${f.key} missing at "${path.join('.')}" — using default`);
                    }
                }
            }
            vals[s.id] = obj;
        }
        failed.value = false;
    } catch (e) {
        failed.value = true;
        if (e.status) toast.error('Failed to load settings: ' + e.message);
    } finally {
        loading.value = false;
    }
}

function normalize(f, v) {
    switch (f.type) {
        case 'number': {
            const n = Number(v);
            return Number.isFinite(n) ? n : 0;
        }
        case 'tags': return Array.isArray(v) ? v : [];
        case 'headers': return v && typeof v === 'object' ? v : {};
        default: return v === null || v === undefined ? '' : v;
    }
}

function nest(obj) {
    const out = {};
    for (const [k, v] of Object.entries(obj)) {
        const path = k.split('.');
        let o = out;
        for (let i = 0; i < path.length - 1; i += 1) {
            const seg = path[i];
            if (!o[seg]) o[seg] = {};
            o = o[seg];
        }
        o[path[path.length - 1]] = v;
    }
    return out;
}

function wrapRoot(section, obj) {
    const root = section.root;
    if (!root.length) return obj;
    const out = {};
    let o = out;
    for (let i = 0; i < root.length - 1; i += 1) {
        o[root[i]] = {};
        o = o[root[i]];
    }
    o[root[root.length - 1]] = obj;
    return out;
}

async function save(section) {
    const source = vals[section.id];
    const obj = {};
    for (const f of section.fields) {
        obj[f.key] = normalize(f, source[f.key]);
    }
    const patch = wrapRoot(section, nest(obj));
    saving.add(section.id);
    try {
        await settingsApi.set('global', patch);
        toast.success(`${section.title} saved`);
        await load();
    } catch (e) {
        toast.error('Failed to save: ' + e.message);
    } finally {
        saving.delete(section.id);
    }
}

onMounted(load);
</script>

<template>
    <section class="page">
        <header class="page-head">
            <div>
                <p class="eyebrow">Configuration</p>
                <h1>System settings</h1>
                <p class="lead">Edit the global <span class="mono">config.json</span>. Every card saves only its own section — other fields stay untouched.</p>
            </div>
            <div class="head-actions">
                <button class="btn btn-ghost" @click="load"><i class="fas fa-rotate" :class="{ 'fa-spin': loading }" /> Reload</button>
            </div>
        </header>

        <div v-if="failed" class="card empty">
            <i class="fas fa-plug-circle-xmark e-icon" />
            <p>Failed to load settings</p>
            <button class="btn btn-primary" @click="load">Retry</button>
        </div>

        <div v-else-if="loading" class="card empty"><span class="spinner" /></div>

        <div v-else>
            <div v-for="s in sections" :key="s.id" class="card">
                <div class="card-head">
                    <div>
                        <h3>{{ s.title }}</h3>
                        <p v-if="s.hint" class="hint">{{ s.hint }}</p>
                    </div>
                    <button class="btn btn-primary btn-sm" :disabled="saving.has(s.id)" @click="save(s)">
                        <span v-if="saving.has(s.id)" class="spinner" style="width:12px;height:12px" />
                        <i v-else class="fas fa-check" /> Save
                    </button>
                </div>

                <div class="card-body">
                    <div class="form-grid">
                        <template v-for="f in s.fields" :key="f.key">
                            <div v-if="f.type === 'bool'" class="bool-cell">
                                <Toggle :model-value="vals[s.id][f.key]" :label="f.label" @update:model-value="vals[s.id][f.key] = $event" />
                                <p v-if="f.help" class="field-help">{{ f.help }}</p>
                            </div>

                            <div v-else-if="f.type === 'tags'" class="field span-2">
                                <label>{{ f.label }}</label>
                                <TagsEditor v-model="vals[s.id][f.key]" :placeholder="f.placeholder" />
                            </div>

                            <div v-else-if="f.type === 'headers'" class="field span-2">
                                <label>{{ f.label }}</label>
                                <HeadersEditor v-model="vals[s.id][f.key]" />
                            </div>

                            <div v-else class="field span-2">
                                <label>{{ f.label }}</label>
                                <textarea
                                    v-if="f.type === 'textarea'"
                                    v-model="vals[s.id][f.key]"
                                    class="textarea"
                                    rows="4"
                                    spellcheck="false"
                                />
                                <select
                                    v-else-if="f.type === 'select'"
                                    v-model="vals[s.id][f.key]"
                                    class="select"
                                >
                                    <option v-for="opt in f.options" :key="opt" :value="opt">{{ opt }}</option>
                                </select>
                                <input
                                    v-else
                                    v-model="vals[s.id][f.key]"
                                    class="input"
                                    :type="f.type === 'password' ? 'password' : 'text'"
                                    :placeholder="f.placeholder || ''"
                                    autocomplete="off"
                                    spellcheck="false"
                                />
                            </div>
                        </template>
                    </div>
                </div>
            </div>
        </div>
    </section>
</template>

<style scoped>
.bool-cell {
    display: flex;
    flex-direction: column;
    gap: 3px;
    padding: 8px 0;
}

.field-help {
    font-size: 12px;
    color: var(--text-3);
    padding-left: 50px;
    max-width: 340px;
}
</style>
