import axios from 'axios';
import { getToken, clearSession } from './auth.js';

const http = axios.create({
    baseURL: '/api',
    timeout: 15000
});

http.interceptors.request.use((config) => {
    const token = getToken();
    if (token) {
        config.headers['X-Token'] = token;
    }
    // Backend expects a Unix timestamp in *seconds* with a ±30 min tolerance.
    config.headers['X-Timestamp'] = Math.floor(Date.now() / 1000);
    return config;
});

http.interceptors.response.use(
    (response) => response.data,
    (error) => {
        const status = error.response ? error.response.status : 0;
        const payload = error.response && error.response.data;
        const message = (payload && payload.message) || error.message || 'Network error';

        if (status === 401) {
            clearSession();
            window.dispatchEvent(new CustomEvent('auth:expired'));
        }

        const err = new Error(message);
        err.status = status;
        return Promise.reject(err);
    }
);

export default http;
