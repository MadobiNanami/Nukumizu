const KEY = 'nukumizu_theme';

function systemPrefersDark() {
    return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
}

export function applyTheme(theme) {
    const dark = theme === 'dark' || (theme !== 'light' && systemPrefersDark());
    document.documentElement.setAttribute('data-theme', dark ? 'dark' : 'light');
    return dark ? 'dark' : 'light';
}

export function currentTheme() {
    return document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light';
}

export function initTheme() {
    const saved = localStorage.getItem(KEY);
    applyTheme(saved || 'system');
}

export function setTheme(theme) {
    localStorage.setItem(KEY, theme);
    return applyTheme(theme);
}

export function toggleTheme() {
    const next = currentTheme() === 'dark' ? 'light' : 'dark';
    localStorage.setItem(KEY, next);
    applyTheme(next);
    return next;
}
