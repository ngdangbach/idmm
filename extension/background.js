const NATIVE_HOST = "com.idmm.downloader";
const IDMM_API = "http://127.0.0.1:8989";

// Default settings
let interceptEnabled = true;

chrome.storage.local.get(['interceptEnabled'], (result) => {
  if (result.interceptEnabled !== undefined) {
    interceptEnabled = result.interceptEnabled;
  }
});

// Setup Context Menus on install
chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.create({
    id: "idmm-download-link",
    title: "⚡ Download with IDMM",
    contexts: ["link", "video", "audio", "image"]
  });

  chrome.contextMenus.create({
    id: "idmm-open-dashboard",
    title: "🚀 Open IDMM Dashboard",
    contexts: ["all"]
  });
});

// Handle Context Menu clicks
chrome.contextMenus.onClicked.addListener((info, tab) => {
  if (info.menuItemId === "idmm-download-link") {
    const targetUrl = info.linkUrl || info.srcUrl || info.pageUrl;
    if (targetUrl) {
      dispatchToIDMM(targetUrl, "");
    }
  } else if (info.menuItemId === "idmm-open-dashboard") {
    chrome.tabs.create({ url: IDMM_API });
  }
});

// Intercept browser downloads
chrome.downloads.onCreated.addListener((downloadItem) => {
  if (!interceptEnabled) return;

  // Avoid intercepting internal or data URLs
  if (!downloadItem.url || downloadItem.url.startsWith("blob:") || downloadItem.url.startsWith("data:")) {
    return;
  }

  // Cancel native browser download
  chrome.downloads.cancel(downloadItem.id, () => {
    chrome.downloads.erase({ id: downloadItem.id }, () => {});
  });

  // Forward to IDMM
  dispatchToIDMM(downloadItem.url, downloadItem.filename || "");
});

// Dispatch download to IDMM (Tries REST API, falls back to Native Messaging)
async function dispatchToIDMM(url, filename) {
  try {
    // 1. Try direct REST API to running IDMM Desktop App
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
      notifyUser("IDMM Download Started", `Accelerating download: ${filename || url.split('/').pop()}`);
      return;
    }
  } catch (err) {
    console.warn("IDMM REST API direct call failed, attempting Native Messaging Host...", err);
  }

  // 2. Fallback to Native Messaging Host
  try {
    chrome.runtime.sendNativeMessage(
      NATIVE_HOST,
      { action: "download", url: url, filename: filename, connections: 16 },
      (response) => {
        if (chrome.runtime.lastError || !response || response.status !== "ok") {
          console.error("Native Messaging Host error:", chrome.runtime.lastError);
          notifyUser("IDMM Offline", "Please launch IDMM application to accelerate your download.");
        } else {
          notifyUser("IDMM Download Dispatched", `Task sent to IDMM: ${filename || url.split('/').pop()}`);
        }
      }
    );
  } catch (e) {
    console.error("Failed to invoke native messaging:", e);
  }
}

function notifyUser(title, message) {
  // Can use chrome.notifications if permission available, or log
  console.log(`[IDMM Notification] ${title}: ${message}`);
}

// Handle messages from content script (Video Sniffer)
chrome.runtime.onMessage.addListener((request, sender, sendResponse) => {
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
