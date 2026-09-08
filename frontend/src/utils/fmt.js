// Severity levels as sent by the backend log websocket (see postLog package):
// 0=DEBUG 1=INFO 2=WARN 3=ERROR 4=FATAL
export const LOG_LEVELS = [
    { key: 'DEBUG', value: 0 },
    { key: 'INFO', value: 1 },
    { key: 'WARN', value: 2 },
    { key: 'ERROR', value: 3 },
    { key: 'FATAL', value: 4 }
];

export const levelOf = (value) => LOG_LEVELS.find((l) => l.value === value) || { key: 'LOG', value };

export function formatBytes(n, digits = 1) {
    if (n === null || n === undefined || Number.isNaN(n)) return '-';
    if (n < 1024) return `${Math.round(n)} B`;
    const units = ['KB', 'MB', 'GB', 'TB', 'PB'];
    let v = n;
    let u = -1;
    do {
        v /= 1024;
        u += 1;
    } while (v >= 1024 && u < units.length - 1);
    return `${v.toFixed(digits)} ${units[u]}`;
}

export function formatPercent(used, total) {
    if (!total) return '—';
    return `${Math.min(100, Math.max(0, Math.round((used / total) * 100)))}%`;
}

export function shortUUID(uuid = '') {
    if (!uuid) return '';
    if (uuid.length <= 13) return uuid;
    return `${uuid.slice(0, 8)}…${uuid.slice(-4)}`;
}

// Online/offline counts + aggregate online ratio. Report arrays carry the
// report; when absent the server has no live telemetry yet.
export function sumUp(entries) {
    const total = entries.length;
    const online = entries.filter((e) => e.online).length;
    return { total, online, offline: total - online };
}
