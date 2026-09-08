import { reactive } from 'vue';

let seq = 0;

export const toasts = reactive([]);

function dismiss(id) {
    const idx = toasts.findIndex((t) => t.id === id);
    if (idx !== -1) toasts[idx].leaving = true;
    setTimeout(() => {
        const i = toasts.findIndex((t) => t.id === id);
        if (i !== -1) toasts.splice(i, 1);
    }, 240);
}

function push(type, message, timeout) {
    const id = ++seq;
    toasts.push({ id, type, message, leaving: false });
    if (timeout > 0) {
        setTimeout(() => dismiss(id), timeout);
    }
    return id;
}

export const toast = {
    success(message, timeout = 3600) {
        return push('success', message, timeout);
    },
    error(message, timeout = 5200) {
        return push('error', message, timeout);
    },
    info(message, timeout = 3200) {
        return push('info', message, timeout);
    },
    warn(message, timeout = 4200) {
        return push('warn', message, timeout);
    },
    dismiss
};
