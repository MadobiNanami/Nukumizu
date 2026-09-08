<script setup>
defineProps({
    open: { type: Boolean, default: false },
    title: { type: String, default: '' },
    wide: { type: Boolean, default: false }
});

const emit = defineEmits(['close']);
</script>

<template>
    <Teleport to="body">
        <Transition name="fade-switch">
            <div v-if="open" class="modal-mask" @click.self="emit('close')">
                <div class="modal" :class="{ wide }" role="dialog" aria-modal="true" @keydown.esc="emit('close')">
                    <div class="modal-head">
                        <h3>{{ title }}</h3>
                        <button class="icon-btn" title="Close" @click="emit('close')">
                            <i class="fas fa-xmark" />
                        </button>
                    </div>
                    <div class="modal-body">
                        <slot />
                    </div>
                    <div v-if="$slots.foot" class="modal-foot">
                        <slot name="foot" />
                    </div>
                </div>
            </div>
        </Transition>
    </Teleport>
</template>
