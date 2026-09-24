<script setup>
import { reactive, ref, watch } from 'vue';
import { settingsApi } from '../api/index.js';
import { debugMode } from '../utils/runtime.js';
import { toast } from '../utils/toast.js';
import Toggle from './Toggle.vue';
import TagsEditor from './TagsEditor.vue';
import HeadersEditor from './HeadersEditor.vue';

// One card of the global config.json: it renders the fields a section
// descriptor declares and saves exactly those fields, leaving every other key
// of the file untouched. Settings.vue and WebHooks.vue both compose this
// component, which is why the descriptor (not the config layout) is what a view
// supplies here.
//
// A descriptor is:
//   id      unique key of the section, used for logging
//   title   card heading
//   hint    optional line under the heading
//   root    path in config.json the fields live under, e.g. ['controllerMethod', 'ntfy']
//   fields  [{ key, type, label, ... }], where type is one of
//           bool | text | password | number | select | textarea | tags | headers
//           - `lp` overrides the field key with an explicit path inside root
//           - `options` lists the choices of a select, `placeholder`/`help` are
//             passed through to the input
const props = defineProps({
    section: { type: Object, required: true },
    config: { type: Object, default: () => ({}) }
});

const emit = defineEmits(['saved']);

const vals = ref({});
const saving = ref(false);

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

// Mirror wrapRoot() on save: prepend the section's root path so a value is read
// from the same place it is written to.
function read() {
    const obj = {};
    for (const f of props.section.fields) {
        const path = [...props.section.root, ...fieldPath(f)];
        obj[f.key] = getVal(props.config, path, defaults(f));
        if (debugMode.value) {
            console.log(`[ConfigSection] Loaded ${props.section.id}.${f.key}:`, obj[f.key]);
            if (!hasVal(props.config, path)) {
                console.warn(`[ConfigSection] ${props.section.id}.${f.key} missing at "${path.join('.')}" — using default`);
            }
        }
    }
    vals.value = obj;
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

async function save() {
    const obj = {};
    for (const f of props.section.fields) {
        obj[f.key] = normalize(f, vals.value[f.key]);
    }
    const patch = wrapRoot(props.section, nest(obj));
    saving.value = true;
    try {
        await settingsApi.set('global', patch);
        toast.success(`${props.section.title} saved`);
        emit('saved');
    } catch (e) {
        toast.error('Failed to save: ' + e.message);
    } finally {
        saving.value = false;
    }
}

// Re-read whenever the caller reloads the configuration, so the card always
// shows what the file holds.
watch(() => props.config, read, { immediate: true });
</script>

<template>
    <div class="card">
        <div class="card-head">
            <div>
                <h3>{{ section.title }}</h3>
                <p v-if="section.hint" class="hint">{{ section.hint }}</p>
            </div>
            <button class="btn btn-primary btn-sm" :disabled="saving" @click="save">
                <span v-if="saving" class="spinner" style="width:12px;height:12px" />
                <i v-else class="fas fa-check" /> Save
            </button>
        </div>

        <div class="card-body">
            <div class="form-grid">
                <template v-for="f in section.fields" :key="f.key">
                    <div v-if="f.type === 'bool'" class="bool-cell">
                        <Toggle :model-value="vals[f.key]" :label="f.label" @update:model-value="vals[f.key] = $event" />
                        <p v-if="f.help" class="field-help">{{ f.help }}</p>
                    </div>

                    <div v-else-if="f.type === 'tags'" class="field span-2">
                        <label>{{ f.label }}</label>
                        <TagsEditor v-model="vals[f.key]" :placeholder="f.placeholder" />
                    </div>

                    <div v-else-if="f.type === 'headers'" class="field span-2">
                        <label>{{ f.label }}</label>
                        <HeadersEditor v-model="vals[f.key]" />
                    </div>

                    <div v-else class="field span-2">
                        <label>{{ f.label }}</label>
                        <textarea
                            v-if="f.type === 'textarea'"
                            v-model="vals[f.key]"
                            class="textarea"
                            rows="4"
                            spellcheck="false"
                        />
                        <select
                            v-else-if="f.type === 'select'"
                            v-model="vals[f.key]"
                            class="select"
                        >
                            <option v-for="opt in f.options" :key="opt" :value="opt">{{ opt }}</option>
                        </select>
                        <input
                            v-else
                            v-model="vals[f.key]"
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
