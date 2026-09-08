<script setup>
import { reactive, watch } from 'vue';

const props = defineProps({
    modelValue: { type: Object, default: () => ({}) }
});

const emit = defineEmits(['update:modelValue']);

const rows = reactive([]);

let suppress = false;

function sync() {
    rows.length = 0;
    for (const [k, v] of Object.entries(props.modelValue || {})) {
        if (k !== '') rows.push({ k, v: v === null || v === undefined ? '' : String(v) });
    }
    if (!rows.length) rows.push({ k: '', v: '' });
}

function commit() {
    const obj = {};
    for (const r of rows) {
        const key = r.k.trim();
        if (key) obj[key] = r.v;
    }
    // Emitting updates the parent model, which flows back through the prop and
    // would otherwise rebuild the rows mid-typing (and drop focus). Suppress
    // that self-echo for this tick.
    suppress = true;
    emit('update:modelValue', obj);
    setTimeout(() => { suppress = false; }, 0);
}

function addRow() {
    if (rows.length && !rows[rows.length - 1].k.trim()) return;
    rows.push({ k: '', v: '' });
}

function removeRow(i) {
    rows.splice(i, 1);
    if (!rows.length) rows.push({ k: '', v: '' });
    commit();
}

// Re-sync only when the value is replaced externally (e.g. after reload).
watch(() => props.modelValue, () => {
    if (!suppress) sync();
}, { deep: false });

sync();
</script>

<template>
    <div class="kv-editor">
        <div v-for="(r, i) in rows" :key="i" class="kv-row">
            <input v-model="r.k" class="input kv-key mono" placeholder="Header name" @input="commit" />
            <input v-model="r.v" class="input kv-val mono" placeholder="Value" @input="commit" />
            <button type="button" class="icon-btn danger" title="Remove header" @click="removeRow(i)">
                <i class="fas fa-minus" />
            </button>
        </div>
        <button type="button" class="btn btn-ghost btn-sm kv-add" @click="addRow">
            <i class="fas fa-plus" /> Add header
        </button>
    </div>
</template>

<style scoped>
.kv-editor {
    display: flex;
    flex-direction: column;
    gap: 8px;
}

.kv-row {
    display: grid;
    grid-template-columns: 1fr 1.4fr 34px;
    gap: 8px;
    align-items: center;
}

.kv-key {
    color: var(--text);
}

.kv-add {
    align-self: flex-start;
}
</style>
