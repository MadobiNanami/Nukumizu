<script setup>
defineProps({
    modelValue: { type: Boolean, default: false },
    label: { type: String, default: '' },
    disabled: { type: Boolean, default: false }
});

const emit = defineEmits(['update:modelValue']);
</script>

<template>
    <label class="switch" :class="{ disabled }">
        <input
            type="checkbox"
            :checked="modelValue"
            :disabled="disabled"
            @change="emit('update:modelValue', $event.target.checked)"
        />
        <span class="track" />
        <span v-if="label" class="switch-label">{{ label }}</span>
    </label>
</template>

<style scoped>
.switch.disabled {
    opacity: 0.5;
    cursor: not-allowed;
}
</style>
