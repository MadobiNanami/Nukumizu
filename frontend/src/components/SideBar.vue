<script setup>
const groups = [
    {
        label: 'Operations',
        items: [
            { name: 'Overview', to: '/overview', title: 'Nodes', icon: 'fa-server' },
            { name: 'Trusted', to: '/trusted', title: 'Trust', icon: 'fa-shield-halved' }
        ]
    },
    {
        label: 'System',
        items: [
            { name: 'Settings', to: '/settings', title: 'Settings', icon: 'fa-sliders' },
            { name: 'Logs', to: '/logs', title: 'Logs', icon: 'fa-terminal' }
        ]
    }
];
</script>

<template>
    <aside class="sidebar">
        <nav class="nav">
            <div v-for="group in groups" :key="group.label" class="group">
                <p class="group-label">{{ group.label }}</p>
                <router-link
                    v-for="item in group.items"
                    :key="item.name"
                    :to="item.to"
                    class="nav-item"
                    active-class="active"
                >
                    <i class="fas" :class="item.icon" aria-hidden="true" />
                    <span>{{ item.title }}</span>
                </router-link>
            </div>
        </nav>

        <div class="side-foot">
            <span class="foot-dot" />
            <span>Nukumizu Console</span>
        </div>
    </aside>
</template>

<style scoped>
.sidebar {
    position: fixed;
    top: 52px;
    left: 0;
    bottom: 0;
    width: 232px;
    z-index: 30;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    padding: 16px 12px;
    background: var(--surface);
    border-right: 1px solid var(--line);
}

.group-label {
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: var(--text-3);
    margin: 4px 10px 6px;
}

.group + .group {
    margin-top: 18px;
}

.nav-item {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 9px 12px;
    border-radius: var(--r-m);
    color: var(--text-2);
    font-size: 14px;
    font-weight: 500;
    transition: background 0.14s ease, color 0.14s ease;
    position: relative;
}

.nav-item i {
    width: 17px;
    text-align: center;
    font-size: 14px;
    color: var(--text-3);
    transition: color 0.14s ease;
}

.nav-item:hover {
    background: var(--surface-3);
    color: var(--text);
}

.nav-item:hover i {
    color: var(--text-2);
}

.nav-item.active {
    background: var(--accent-soft);
    color: var(--accent);
    font-weight: 600;
}

.nav-item.active i {
    color: var(--accent);
}

.nav-item.active::before {
    content: '';
    position: absolute;
    left: -12px;
    top: 8px;
    bottom: 8px;
    width: 3px;
    border-radius: 0 3px 3px 0;
    background: var(--accent);
}

.side-foot {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 10px 6px 2px;
    font-size: 11.5px;
    color: var(--text-3);
}

.foot-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--ok);
    box-shadow: 0 0 0 3px var(--ok-soft);
}
</style>
