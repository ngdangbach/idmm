const NATIVE_HOST = "com.idmm.downloader";
const IDMM_API = "http://127.0.0.1:8989";

// Compatible API reference (browser for Firefox, fallback to chrome)
const ext = typeof browser !== "undefined" ? browser : chrome;

let interceptEnabled = true;

// Load settings
if (ext.storage && ext.storage.local) {
  ext.storage.local.get(['interceptEnabled'], (result) => {
    if (result && result.interceptEnabled !== undefined) {
      interceptEnabled = result.interceptEnabled;
    }
  });
}

// Setup Context Menus
function setupMenus() {
  const menuAPI = ext.contextMenus || ext.menus;
  if (!menuAPI) return;

  try {
    menuAPI.removeAll(() => {
      menuAPI.create({
        id: "idmm-download-link",
        title: "⚡ Download with IDMM",
        contexts: ["link", "video", "audio", "image"]
      });

      menuAPI.create({
        id: "idmm-open-dashboard",
        title: "🚀 Open IDMM Dashboard",
        contexts: ["all"]
      });
    });
  } catch (e) {
    // Menu item might already exist or API not ready
  }
}

if (ext.runtime && ext.runtime.onInstalled) {
  ext.runtime.onInstalled.addListener(setupMenus);
}
setupMenus();

// Handle Context Menu clicks
const menuAPI = ext.contextMenus || ext.menus;
if (menuAPI && menuAPI.onClicked) {
  menuAPI.onClicked.addListener((info, tab) => {
    if (info.menuItemId === "idmm-download-link") {
      const targetUrl = info.linkUrl || info.srcUrl || info.pageUrl;
      if (targetUrl) {
        dispatchToIDMM(targetUrl, "");
      }
    } else if (info.menuItemId === "idmm-open-dashboard") {
      ext.tabs.create({ url: IDMM_API });
    }
  });
}

// Intercept browser downloads
if (ext.downloads && ext.downloads.onCreated) {
  ext.downloads.onCreated.addListener((downloadItem) => {
    if (!interceptEnabled) return;

    if (!downloadItem.url || downloadItem.url.startsWith("blob:") || downloadItem.url.startsWith("data:")) {
      return;
    }

    // Cancel native browser download
    try {
      if (ext.downloads.cancel) {
        const cancelPromise = ext.downloads.cancel(downloadItem.id);
        if (cancelPromise && typeof cancelPromise.then === "function") {
          cancelPromise.then(() => {
            if (ext.downloads.erase) ext.downloads.erase({ id: downloadItem.id });
          }).catch(() => {});
        } else {
          ext.downloads.cancel(downloadItem.id, () => {
            if (ext.downloads.erase) ext.downloads.erase({ id: downloadItem.id }, () => {});
          });
        }
      }
    } catch (e) {
      console.warn("Could not cancel native download:", e);
    }

    // Forward to IDMM
    dispatchToIDMM(downloadItem.url, downloadItem.filename || "");
  });
}

// Send Native Message helper supporting Promise and Callback
function sendNative(host, message) {
  return new Promise((resolve, reject) => {
    try {
      if (typeof browser !== "undefined" && browser.runtime && browser.runtime.sendNativeMessage) {
        browser.runtime.sendNativeMessage(host, message)
          .then(resolve)
          .catch(reject);
      } else if (typeof chrome !== "undefined" && chrome.runtime && chrome.runtime.sendNativeMessage) {
        chrome.runtime.sendNativeMessage(host, message, (response) => {
          if (chrome.runtime.lastError) {
            reject(chrome.runtime.lastError);
          } else {
            resolve(response);
          }
        });
      } else {
        reject(new Error("Native messaging not supported"));
      }
    } catch (err) {
      reject(err);
    }
  });
}

// Dispatch download to IDMM (REST API -> fallback to Native Messaging)
async function dispatchToIDMM(url, filename) {
  try {
    const res = await fetch(`${IDMM_API}/api/tasks`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        url: url,
        target_path: filename,
        connections: 16
      })
    });

    if (res.ok) {
      console.log(`[IDMM] Download started via REST API: ${filename || url}`);
      return;
    }
  } catch (err) {
    console.warn("[IDMM] REST API offline, attempting Native Messaging Host...", err);
  }

  try {
    const response = await sendNative(NATIVE_HOST, {
      action: "download",
      url: url,
      filename: filename,
      connections: 16
    });

    if (!response || response.status !== "ok") {
      console.warn("[IDMM] Native host reported error:", response);
    } else {
      console.log("[IDMM] Download dispatched successfully via Native Host!");
    }
  } catch (e) {
    console.error("[IDMM] Failed to invoke native messaging:", e);
  }
}

// Handle messages from content script (Video Sniffer)
ext.runtime.onMessage.addListener((request, sender, sendResponse) => {
  if (request.action === "download_media") {
    dispatchToIDMM(request.url, request.filename || "");
    sendResponse({ status: "ok" });
  } else if (request.action === "get_status") {
    fetch(`${IDMM_API}/api/tasks`)
      .then(r => r.json())
      .then(tasks => sendResponse({ status: "connected", tasks: tasks }))
      .catch(() => sendResponse({ status: "offline" }));
    return true; // Keep message channel open for async response
  }
});
