const api = document.getElementById('api'); const status = document.getElementById('status');
chrome.storage.local.get({ apiBase: 'https://192.168.0.130:8080' }).then(({ apiBase }) => { api.value = apiBase; });
document.getElementById('save').addEventListener('click', async () => { const value = api.value.trim().replace(/\/$/, ''); if (!/^https?:\/\//i.test(value)) { status.textContent = '请输入 http 或 https 地址'; return; } await chrome.storage.local.set({ apiBase: value }); status.textContent = '已保存'; });
