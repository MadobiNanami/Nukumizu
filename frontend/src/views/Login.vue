<script setup>
import { reactive, ref, computed } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { authApi } from '../api/index.js';
import { setSession } from '../utils/auth.js';
import { toast } from '../utils/toast.js';

const router = useRouter();
const route = useRoute();

const mode = ref('login');
const loading = ref(false);
const showPw = ref(false);
const form = reactive({ username: '', password: '' });
const errMsg = ref('');

const canSubmit = computed(() => form.username.trim().length > 0 && form.password.length >= 6);

function goTo(target) {
    mode.value = target;
    errMsg.value = '';
}

function afterLogin(token, user) {
    setSession(token, user);
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/overview';
    router.push(redirect);
}

async function submit() {
    if (!canSubmit.value || loading.value) return;
    loading.value = true;
    errMsg.value = '';
    try {
        if (mode.value === 'login') {
            const res = await authApi.login(form.username.trim(), form.password);
            afterLogin(res.token, {
                userID: res.userID,
                username: res.username,
                level: res.level,
                registerDate: res.registerDate
            });
        } else {
            const res = await authApi.register(form.username.trim(), form.password);
            afterLogin(res.token, {
                userID: res.userID,
                username: res.username,
                level: res.level
            });
            toast.success('Admin created — welcome to Nukumizu');
        }
    } catch (e) {
        if (mode.value === 'register' && e.status === 403) {
            errMsg.value = 'An admin already exists — registration is closed. Please sign in.';
            mode.value = 'login';
        } else {
            errMsg.value = e.message || 'Operation failed';
        }
    } finally {
        loading.value = false;
    }
}
</script>

<template>
    <div class="auth">
        <div class="auth-inner">
            <div class="brand">
                <span class="mark">
                    <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 2 3 6.5v3.1C3 15.2 6.9 20 12 21c5.1-1 9-5.8 9-11.4V6.5L12 2Zm0 7.2V19c-3.7-.7-6.6-4.4-6.6-8.2V7.6L12 4.4l6.6 3.2v3.2c0 1.9-.5 3.7-1.4 5.1H12Z" fill="currentColor"/></svg>
                </span>
                <h1>Nukumizu</h1>
                <p>Remote server monitoring &amp; alert bot — admin console</p>
            </div>

            <div class="panel">
                <div class="tabs">
                    <button :class="{ on: mode === 'login' }" @click="goTo('login')">Sign in</button>
                    <button :class="{ on: mode === 'register' }" @click="goTo('register')">Create admin</button>
                </div>

                <form class="fields" @submit.prevent="submit">
                    <div class="field">
                        <label for="username">Username</label>
                        <input id="username" v-model="form.username" class="input" autocomplete="username" autofocus />
                    </div>

                    <div class="field">
                        <label for="password">Password</label>
                        <div class="pw">
                            <input
                                id="password"
                                v-model="form.password"
                                class="input"
                                :type="showPw ? 'text' : 'password'"
                                autocomplete="current-password"
                                placeholder="At least 6 characters"
                            />
                            <button type="button" class="icon-btn eye" tabindex="-1" @click="showPw = !showPw">
                                <i class="fas" :class="showPw ? 'fa-eye-slash' : 'fa-eye'" />
                            </button>
                        </div>
                    </div>

                    <p v-if="errMsg" class="form-err">{{ errMsg }}</p>

                    <button class="btn btn-primary submit" type="submit" :disabled="!canSubmit || loading">
                        <span v-if="loading" class="spinner" style="width:14px;height:14px;border-width:2px" />
                        <span>{{ mode === 'login' ? 'Sign in' : 'Create & enter' }}</span>
                    </button>

                    <p v-if="mode === 'register'" class="hint">
                        Only available while no user exists yet. The first account becomes the admin.
                    </p>
                </form>
            </div>
        </div>
    </div>
</template>

<style scoped>
.auth {
    min-height: 100vh;
    display: grid;
    place-items: center;
    padding: 32px 20px;
    background:
        radial-gradient(900px 480px at 15% -10%, var(--accent-soft), transparent 60%),
        radial-gradient(700px 420px at 110% 110%, color-mix(in srgb, var(--ok) 8%, transparent), transparent 60%),
        var(--bg-grad) fixed;
}

.auth-inner {
    width: 100%;
    max-width: 360px;
    display: flex;
    flex-direction: column;
    gap: 22px;
}

.brand {
    text-align: center;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
}

.mark {
    width: 54px;
    height: 54px;
    border-radius: 15px;
    display: grid;
    place-items: center;
    background: linear-gradient(150deg, var(--accent), var(--accent-strong));
    color: #fff;
    box-shadow: 0 10px 30px -8px color-mix(in srgb, var(--accent) 55%, transparent);
    margin-bottom: 4px;
}

.mark svg { width: 32px; height: 32px; }

.brand h1 {
    font-size: 26px;
    letter-spacing: -0.02em;
}

.brand p {
    font-size: 13px;
    color: var(--text-2);
}

.panel {
    background: var(--surface);
    border: 1px solid var(--line);
    border-radius: var(--r-l);
    box-shadow: var(--shadow-2);
    padding: 8px 24px 24px;
}

.tabs {
    display: flex;
    gap: 4px;
    margin: 0 -24px 20px;
    padding: 0 20px;
    border-bottom: 1px solid var(--line);
}

.tabs button {
    position: relative;
    padding: 14px 8px;
    font-size: 14px;
    font-weight: 600;
    color: var(--text-3);
}

.tabs button.on { color: var(--accent); }

.tabs button.on::after {
    content: '';
    position: absolute;
    left: 8px;
    right: 8px;
    bottom: -1px;
    height: 2px;
    border-radius: 2px;
    background: var(--accent);
}

.fields {
    display: flex;
    flex-direction: column;
    gap: 14px;
}

.pw { position: relative; }

.pw .input { padding-right: 40px; }

.pw .eye {
    position: absolute;
    right: 4px;
    top: 50%;
    transform: translateY(-50%);
}

.form-err {
    font-size: 13px;
    color: var(--bad);
    background: var(--bad-soft);
    border-radius: var(--r-s);
    padding: 8px 12px;
}

.submit { width: 100%; padding: 11px; }

.hint {
    font-size: 12.5px;
    color: var(--text-3);
    text-align: center;
}
</style>
