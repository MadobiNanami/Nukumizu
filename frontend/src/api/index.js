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
