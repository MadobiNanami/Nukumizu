<script setup>
import { onMounted, ref } from 'vue';
import { settingsApi } from '../api/index.js';
import { toast } from '../utils/toast.js';
import ConfigSection from '../components/ConfigSection.vue';

// Every section is rendered and saved by ConfigSection, which owns the
// field-descriptor format. The incoming webhook API has its own page (see
// WebHooks.vue) and is not listed here.
const sections = [
    {
        id: 'system',
        title: 'System',
        hint: 'HTTP listener and global runtime switches.',
        root: ['system'],
        fields: [
            { key: 'debugMode', type: 'bool', label: 'Debug mode', help: 'Skipped X-Timestamp checks and verbose debug logging.' },
            { key: 'listenAddr', type: 'text', label: 'Listen address' },
            { key: 'listenPort', type: 'text', label: 'Listen port' },
            { key: 'networkProxy', type: 'text', label: 'Network proxy', placeholder: 'http://host:port' }
        ]
    },
    {
        id: 'debug',
        title: 'Debug logging',
        hint: 'Per-module verbosity for the bot pipes.',
        root: ['debug'],
        fields: [
            { key: 'showNapcatMsg', type: 'bool', label: 'NapCat messages' },
            { key: 'showNapcatAction', type: 'bool', label: 'NapCat actions' },
            { key: 'showTelegramMsg', type: 'bool', label: 'Telegram messages' },
            { key: 'showTriggerCmdEcho', type: 'bool', label: 'Trigger command echo' },
            { key: 'showKomariTaskEcho', type: 'bool', label: 'Komari task echo' },
            { key: 'napcatIgnoreSelfMsg', type: 'bool', label: 'Ignore NapCat self messages' }
        ]
    },
    {
        id: 'komari',
        title: 'Komari dashboard',
        hint: 'Connection the monitor reads node data from. Takes effect on restart.',
        root: ['komari'],
        fields: [
            { key: 'dashboardURL', type: 'text', label: 'Dashboard URL' },
            { key: 'account.username', lp: ['account', 'username'], type: 'text', label: 'Account' },
            { key: 'account.password', lp: ['account', 'password'], type: 'password', label: 'Password' }
        ]
    },
    {
        id: 'controllerMessage',
        title: 'Message templates',
        hint: 'Templates rendered for bot pushes. Placeholders like {{ serverName }} stay as-is.',
        root: ['controllerMessage'],
        fields: [
            { key: 'BOT_STARTED', type: 'textarea', label: 'BOT_STARTED' },
            { key: 'BOT_HELP', type: 'textarea', label: 'BOT_HELP' },
            { key: 'TG_BOT_START', type: 'textarea', label: 'TG_BOT_START' },
            { key: 'SERVER_STATUS_CHANGED', type: 'textarea', label: 'SERVER_STATUS_CHANGED' },
            { key: 'SERVER_LIST', type: 'textarea', label: 'SERVER_LIST' },
            { key: 'SERVER_EXECUTE_RESULT', type: 'textarea', label: 'SERVER_EXECUTE_RESULT' }
        ]
    },
    {
        id: 'qq',
        title: 'QQ controller (NapCat)',
        root: ['controllerMethod', 'qq(napcat)'],
        fields: [
            { key: 'enabled', type: 'bool', label: 'Enabled' },
            { key: 'markdown', type: 'bool', label: 'Markdown', help: 'Send the formatted variant of the templates (code blocks, inline code).' },
            { key: 'networkUseProxy', type: 'bool', label: 'Use network proxy' },
            { key: 'napcatAddr', type: 'text', label: 'NapCat address' },
            { key: 'napcatPort', type: 'text', label: 'NapCat port' },
            { key: 'napcatToken', type: 'password', label: 'NapCat token' },
            { key: 'botQQID', type: 'number', label: 'Bot QQ ID' },
            { key: 'listenMethod', type: 'select', label: 'Listen method', options: ['global', 'at'] }
        ]
    },
    {
        id: 'telegram',
        title: 'Telegram controller',
        root: ['controllerMethod', 'telegram'],
        fields: [
            { key: 'enabled', type: 'bool', label: 'Enabled' },
            { key: 'markdown', type: 'bool', label: 'Markdown', help: 'Sends messages with parse_mode=Markdown; turn off to have text delivered verbatim.' },
            { key: 'networkUseProxy', type: 'bool', label: 'Use network proxy' },
            { key: 'botToken', type: 'password', label: 'Bot token' },
            { key: 'listenMethod', type: 'select', label: 'Listen method', options: ['global', 'at'] }
        ]
    },
    {
        id: 'email',
        title: 'Email notifications',
        root: ['controllerMethod', 'email'],
        fields: [
            { key: 'enabled', type: 'bool', label: 'Enabled' },
            { key: 'markdown', type: 'bool', label: 'Markdown', help: 'Send the formatted variant of the templates (code blocks, inline code).' },
            { key: 'networkUseProxy', type: 'bool', label: 'Use network proxy' },
            { key: 'smtpHost', type: 'text', label: 'SMTP host' },
            { key: 'smtpPort', type: 'number', label: 'SMTP port' },
            { key: 'username', type: 'text', label: 'Username' },
            { key: 'password', type: 'password', label: 'Password' },
            { key: 'from', type: 'text', label: 'From address' },
            { key: 'to', type: 'tags', label: 'Recipients', placeholder: 'Type an address and press Enter' },
            { key: 'useTLS', type: 'bool', label: 'Use TLS' }
        ]
    },
    {
        id: 'ntfy',
        title: 'Ntfy notifications',
        root: ['controllerMethod', 'ntfy'],
        fields: [
            { key: 'enabled', type: 'bool', label: 'Enabled' },
            { key: 'markdown', type: 'bool', label: 'Markdown', help: 'Send the formatted variant of the templates (code blocks, inline code).' },
            { key: 'networkUseProxy', type: 'bool', label: 'Use network proxy' },
            { key: 'server', type: 'text', label: 'Server' },
            { key: 'topic', type: 'text', label: 'Topic' },
            { key: 'token', type: 'password', label: 'Token' },
            { key: 'priority', type: 'select', label: 'Priority', options: ['min', 'low', 'default', 'high', 'urgent', 'max'] }
        ]
    },
    {
        id: 'webhook',
        title: 'Webhook notifications',
        hint: 'Where this program posts its own alerts. The incoming webhook API has its own page (see WebHooks).',
        root: ['controllerMethod', 'webhook'],
        fields: [
            { key: 'enabled', type: 'bool', label: 'Enabled' },
            { key: 'markdown', type: 'bool', label: 'Markdown', help: 'Format the alert body with code blocks and inline code in the posted payload.' },
            { key: 'networkUseProxy', type: 'bool', label: 'Use network proxy' },
            { key: 'url', type: 'text', label: 'URL' },
            { key: 'method', type: 'select', label: 'Method', options: ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'] },
            { key: 'headers', type: 'headers', label: 'Headers' },
            { key: 'template', type: 'textarea', label: 'Payload template' }
        ]
    },
    {
        id: 'paths',
        title: 'Storage paths',
        hint: 'Where runtime data lives. Takes effect on restart.',
        root: [],
        fields: [
            { key: 'dataPath', type: 'text', label: 'Data path' },
            { key: 'dbPath', type: 'text', label: 'Database path' }
        ]
    }
];

const loading = ref(true);
const failed = ref(false);
const cfg = ref({});

async function load() {
    try {
        const res = await settingsApi.get('global');
        cfg.value = (res && res.data && res.data.config) || {};
        failed.value = false;
    } catch (e) {
        failed.value = true;
        if (e.status) toast.error('Failed to load settings: ' + e.message);
    } finally {
        loading.value = false;
    }
}

onMounted(load);
</script>

<template>
    <section class="page">
        <header class="page-head">
            <div>
                <p class="eyebrow">Configuration</p>
                <h1>System settings</h1>
                <p class="lead">Edit the global <span class="mono">config.json</span>. Every card saves only its own section — other fields stay untouched.</p>
            </div>
            <div class="head-actions">
                <button class="btn btn-ghost" @click="load"><i class="fas fa-rotate" :class="{ 'fa-spin': loading }" /> Reload</button>
            </div>
        </header>

        <div v-if="failed" class="card empty">
            <i class="fas fa-plug-circle-xmark e-icon" />
            <p>Failed to load settings</p>
            <button class="btn btn-primary" @click="load">Retry</button>
        </div>

        <div v-else-if="loading" class="card empty"><span class="spinner" /></div>

        <div v-else>
            <ConfigSection v-for="s in sections" :key="s.id" :section="s" :config="cfg" @saved="load" />
        </div>
    </section>
</template>
