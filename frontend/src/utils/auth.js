import { reactive } from 'vue';

const TOKEN_KEY = 'nukumizu_token';
const USER_KEY = 'nukumizu_user';

function readUser() {
    try {
        return JSON.parse(localStorage.getItem(USER_KEY)) || null;
    } catch {
        return null;
    }
}

export const auth = reactive({
    token: localStorage.getItem(TOKEN_KEY) || '',
    user: readUser()
});

export function setSession(token, user) {
    auth.token = token;
    auth.user = user;
    localStorage.setItem(TOKEN_KEY, token);
    localStorage.setItem(USER_KEY, JSON.stringify(user));
}

export function clearSession() {
    auth.token = '';
    auth.user = null;
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USER_KEY);
}

export function getToken() {
    return auth.token;
}

export function isLoggedIn() {
    return !!auth.token;
}
