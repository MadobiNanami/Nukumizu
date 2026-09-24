import http from '../utils/http.js';

export const authApi = {
    login: (username, password) => http.post('/user/login', { username, password }),
    register: (username, password) => http.post('/user/register', { username, password })
};

// /api/server/getStatus?uuid=all → { success, message, data: { "<uuid>": { uuid, name, online, report } } }
export const serverApi = {
    statusAll: () => http.get('/server/getStatus?uuid=all'),
    infoAll: () => http.get('/server/getInfo?uuid=all')
};

// /api/settings/get?type=… / /api/settings/set?type=…
// get → { success, message, data: { config } }.
// `type` is one of global | bot_user_config | bot_node_config.
// For set, pass a partial object; a JSON null value removes that key.
export const settingsApi = {
    get: (type) => http.get(`/settings/get?type=${encodeURIComponent(type)}`),
    set: (type, patch) => http.post(`/settings/set?type=${encodeURIComponent(type)}`, patch)
};

// Incoming webhook endpoints (admin). They live in the `webhook.endpoints`
// section of config.json, but are managed here rather than through the settings
// API because they are a keyed collection: add/modify take one endpoint object
// and change only the fields they carry, and a new endpoint is rejected with
// 409 when its name is taken.
// list → { success, message, data: { endpoints: { "<name>": { enabled, token, notifyPipes } } } }.
export const webhookApi = {
    list: () => http.get('/webhook/list'),
    add: (endpoint) => http.post('/webhook/add', endpoint),
    modify: (endpoint) => http.post('/webhook/modify', endpoint),
    remove: (name) => http.post('/webhook/delete', { name })
};
