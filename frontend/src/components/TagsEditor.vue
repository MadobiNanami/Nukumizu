<script setup>
import { ref } from 'vue';

const props = defineProps({
    modelValue: { type: Array, default: () => [] },
    placeholder: { type: String, default: 'Type and press Enter to add' }
});

const emit = defineEmits(['update:modelValue']);
const text = ref('');

function add() {
    const items = text.value
        .split(/[,\n]/)
        .map((s) => s.trim())
        .filter(Boolean);
    if (!items.length) return;
    const next = [...props.modelValue];
    for (const it of items) {
        if (!next.includes(it)) next.push(it);
    }
    emit('update:modelValue', next);
    text.value = '';
}

function removeAt(i) {
    const next = props.modelValue.slice();
    next.splice(i, 1);
    emit('update:modelValue', next);
}

function onKeydown(e) {
    if (e.key === 'Enter' || e.key === ',') {
        e.preventDefault();
        add();
    } else if (e.key === 'Backspace' && !text.value && props.modelValue.length) {
        removeAt(props.modelValue.length - 1);
    }
}
</script>

<template>
    <div class="tags-editor">
        <div v-if="modelValue.length" class="tags">
            <span v-for="(t, i) in modelValue" :key="i" class="tag">
                {{ t }}
                <button type="button" class="tag-x" @click="removeAt(i)"><i class="fas fa-xmark" /></button>
            </span>
        </div>
        <input
            v-model="text"
            class="input tags-input"
            :placeholder="modelValue.length ? placeholder : placeholder"
            @keydown="onKeydown"
            @blur="add"
        />
    </div>
</template>

<style scoped>
.tags-editor {
    display: flex;
    flex-direction: column;
    gap: 8px;
    background: var(--surface-2);
    border: 1px solid var(--line-strong);
    border-radius: var(--r-s);
    padding: 8px;
    transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.tags-editor:focus-within {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-soft);
    background: var(--surface);
}

.tags {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
}

.tag {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 3px 6px 3px 10px;
    background: var(--accent-soft);
    color: var(--accent);
    border-radius: var(--r-pill);
    font-size: 12.5px;
    font-weight: 600;
}

.tag-x {
    width: 16px;
    height: 16px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    color: inherit;
    font-size: 10px;
}

.tag-x:hover { background: color-mix(in srgb, var(--accent) 20%, transparent); }

.tags-input {
    border: 0;
    background: transparent;
    padding: 2px 4px;
    box-shadow: none !important;
}

.tags-input:focus {
    background: transparent;
}
</style>
