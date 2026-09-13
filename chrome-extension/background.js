const ENABLED_ICONS = {
  16: 'icons/icon-16.png',
  32: 'icons/icon-32.png',
  48: 'icons/icon-48.png',
  128: 'icons/icon-128.png',
};

const DISABLED_ICONS = {
  16: 'icons/icon-disabled-16.png',
  32: 'icons/icon-disabled-32.png',
  48: 'icons/icon-disabled-48.png',
  128: 'icons/icon-disabled-128.png',
};

async function syncAction(enabled) {
  await chrome.action.setIcon({ path: enabled ? ENABLED_ICONS : DISABLED_ICONS });
  await chrome.action.setTitle({ title: enabled ? 'FWLOG 日志查询' : 'FWLOG 已停用' });
}

async function restoreAction() {
  const { pluginEnabled = true } = await chrome.storage.local.get({ pluginEnabled: true });
  await syncAction(pluginEnabled !== false);
}

chrome.runtime.onInstalled.addListener(() => { void restoreAction(); });
chrome.runtime.onStartup.addListener(() => { void restoreAction(); });
chrome.storage.onChanged.addListener((changes, areaName) => {
  if (areaName === 'local' && changes.pluginEnabled) {
    void syncAction(changes.pluginEnabled.newValue !== false);
  }
});

void restoreAction();
