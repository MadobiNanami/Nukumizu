import { ref } from 'vue';
import { settingsApi } from '../api/index.js';

// Runtime flags mirrored from the backend config so any view can read them
// without refetching the whole config.

// debugMode mirrors system.debugMode from the global config (config.json).
export const debugMode = ref(false);

// loadDebugMode reads the global config and mirrors system.debugMode into the
// `debugMode` global. The endpoint needs an admin session, so callers only
// invoke it once logged in; a failed request keeps the current value.
export async function loadDebugMode() {
    try {
        const res = await settingsApi.get('global');
        const config = (res && res.data && res.data.config) || {};
        const system = config.system || {};
        debugMode.value = !!system.debugMode;
    } catch {
        // Non-fatal: the flag simply keeps its current value.
    }
}
