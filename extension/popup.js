const IDMM_API = "http://127.0.0.1:8989";
const statusPill = document.getElementById('statusPill');
const statusText = document.getElementById('statusText');
const activeTasksCount = document.getElementById('activeTasksCount');
const currentThroughput = document.getElementById('currentThroughput');
const toggleIntercept = document.getElementById('toggleIntercept');
const btnOpenDashboard = document.getElementById('btnOpenDashboard');

// Load stored intercept toggle
chrome.storage.local.get(['interceptEnabled'], (res) => {
  if (res.interceptEnabled !== undefined) {
    toggleIntercept.checked = res.interceptEnabled;
  }
});

toggleIntercept.onchange = () => {
  chrome.storage.local.set({ interceptEnabled: toggleIntercept.checked });
};

btnOpenDashboard.onclick = () => {
  chrome.tabs.create({ url: IDMM_API });
};

// Check IDMM status
async function checkStatus() {
  try {
    const res = await fetch(`${IDMM_API}/api/tasks`);
    if (res.ok) {
      const tasks = await res.json();
      statusPill.className = "status-indicator status-connected";
      statusText.textContent = "Connected";

      let active = 0;
      let totalSpeed = 0;
      tasks.forEach(t => {
        if (t.status === 'DOWNLOADING') {
          active++;
          totalSpeed += (t.speed_bytes_sec || 0);
        }
      });

      activeTasksCount.textContent = active;
      currentThroughput.textContent = `${(totalSpeed / (1024 * 1024)).toFixed(2)} MB/s`;
      return;
    }
  } catch (e) {}

  statusPill.className = "status-indicator status-offline";
  statusText.textContent = "Offline";
  activeTasksCount.textContent = "0";
  currentThroughput.textContent = "0 MB/s";
}

checkStatus();
setInterval(checkStatus, 2000);
