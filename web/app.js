// State
let tasks = [];
let currentFilter = 'all';
let speedHistory = new Array(30).fill(0);

// Elements
const taskListContainer = document.getElementById('taskListContainer');
const emptyState = document.getElementById('emptyState');
const globalSpeedEl = document.getElementById('globalSpeed');
const globalConnsEl = document.getElementById('globalConns');
const taskCountEl = document.getElementById('taskCount');
const countAllEl = document.getElementById('countAll');
const countActiveEl = document.getElementById('countActive');
const countCompletedEl = document.getElementById('countCompleted');
const searchInput = document.getElementById('searchInput');

// Modals
const newTaskModal = document.getElementById('newTaskModal');
const btnOpenNewTask = document.getElementById('btnOpenNewTask');
const btnCloseModal = document.getElementById('btnCloseModal');
const btnCancelModal = document.getElementById('btnCancelModal');
const newTaskForm = document.getElementById('newTaskForm');
const inputUrl = document.getElementById('inputUrl');
const inputConnections = document.getElementById('inputConnections');
const valConnections = document.getElementById('valConnections');
const btnPasteUrl = document.getElementById('btnPasteUrl');

// Video Player Modal
const videoPlayerModal = document.getElementById('videoPlayerModal');
const btnClosePlayer = document.getElementById('btnClosePlayer');
const html5Player = document.getElementById('html5Player');
const playerVideoTitle = document.getElementById('playerVideoTitle');
const playerStreamUrl = document.getElementById('playerStreamUrl');
const btnCopyStreamUrl = document.getElementById('btnCopyStreamUrl');

// Speed Chart Canvas
const canvas = document.getElementById('speedChart');
const ctx = canvas.getContext('2d');

// Helper: Format Bytes
function formatBytes(bytes) {
  if (!bytes || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let val = bytes;
  let idx = 0;
  while (val >= 1024 && idx < units.length - 1) {
    val /= 1024;
    idx++;
  }
  return `${val.toFixed(2)} ${units[idx]}`;
}

// Helper: Format Duration
function formatDuration(sec) {
  if (!sec || sec <= 0) return '--';
  if (sec < 60) return `${sec}s`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m ${sec % 60}s`;
  return `${Math.floor(sec / 3600)}h ${Math.floor((sec % 3600) / 60)}m`;
}

// Helper: Detect Icon
function getFileIcon(filename, url) {
  if ((url && url.startsWith('magnet:')) || (filename && filename.toLowerCase().includes('torrent'))) return '🧲';
  const ext = (filename || '').split('.').pop().toLowerCase();
  if (ext === 'torrent') return '🧲';
  if (['mp4', 'mkv', 'mov', 'webm', 'ts', 'm3u8', 'avi'].includes(ext)) return '🎬';
  if (['mp3', 'wav', 'flac', 'aac', 'ogg'].includes(ext)) return '🎵';
  if (['zip', 'rar', '7z', 'tar', 'gz', 'iso'].includes(ext)) return '🗜️';
  if (['exe', 'msi', 'dmg', 'apk', 'app'].includes(ext)) return '⚙️';
  if (['pdf', 'docx', 'xlsx', 'pptx', 'txt'].includes(ext)) return '📄';
  return '📦';
}

// Draw Speed Sparkline Chart
function drawSpeedChart() {
  const rect = canvas.getBoundingClientRect();
  const dpr = window.devicePixelRatio || 1;
  const displayWidth = Math.floor(rect.width) || 188;
  const displayHeight = Math.floor(rect.height) || 60;

  if (canvas.width !== Math.floor(displayWidth * dpr) || canvas.height !== Math.floor(displayHeight * dpr)) {
    canvas.width = Math.floor(displayWidth * dpr);
    canvas.height = Math.floor(displayHeight * dpr);
  }

  ctx.save();
  ctx.scale(dpr, dpr);
  const w = displayWidth;
  const h = displayHeight;
  ctx.clearRect(0, 0, w, h);

  const max = Math.max(...speedHistory, 1024 * 1024); // Minimum 1MB max
  const step = w / (speedHistory.length - 1);

  // Gradient fill
  const grad = ctx.createLinearGradient(0, 0, 0, h);
  grad.addColorStop(0, 'rgba(6, 182, 212, 0.4)');
  grad.addColorStop(1, 'rgba(6, 182, 212, 0.0)');

  ctx.beginPath();
  ctx.moveTo(0, h);
  for (let i = 0; i < speedHistory.length; i++) {
    const x = i * step;
    const y = h - (speedHistory[i] / max) * (h - 10) - 5;
    ctx.lineTo(x, y);
  }
  ctx.lineTo(w, h);
  ctx.closePath();
  ctx.fillStyle = grad;
  ctx.fill();

  // Stroke line
  ctx.beginPath();
  for (let i = 0; i < speedHistory.length; i++) {
    const x = i * step;
    const y = h - (speedHistory[i] / max) * (h - 10) - 5;
    if (i === 0) ctx.moveTo(x, y);
    else ctx.lineTo(x, y);
  }
  ctx.strokeStyle = '#06B6D4';
  ctx.lineWidth = 2;
  ctx.shadowColor = 'rgba(6, 182, 212, 0.5)';
  ctx.shadowBlur = 8;
  ctx.stroke();
  ctx.shadowBlur = 0;
  ctx.restore();
}

window.addEventListener('resize', drawSpeedChart);

// Render Tasks
function renderTasks() {
  const query = searchInput.value.toLowerCase().trim();
  const filtered = tasks.filter(t => {
    // Filter by category
    if (currentFilter === 'active' && t.status !== 'DOWNLOADING') return false;
    if (currentFilter === 'completed' && t.status !== 'COMPLETED') return false;
    if (currentFilter === 'video' && !getFileIcon(t.filename, t.url).includes('🎬')) return false;
    if (currentFilter === 'archive' && !getFileIcon(t.filename, t.url).includes('🗜️')) return false;
    if (currentFilter === 'document' && !getFileIcon(t.filename, t.url).includes('📄')) return false;
    if (currentFilter === 'torrent' && !(t.url.startsWith('magnet:') || (t.filename && t.filename.toLowerCase().includes('torrent')) || t.url.endsWith('.torrent') || t.url.startsWith('file://'))) return false;

    // Filter by search
    if (query && !t.filename.toLowerCase().includes(query) && !t.url.toLowerCase().includes(query)) {
      return false;
    }
    return true;
  });

  if (filtered.length === 0) {
    emptyState.style.display = 'flex';
    // Remove previous cards
    const existingCards = taskListContainer.querySelectorAll('.task-card');
    existingCards.forEach(c => c.remove());
    return;
  }

  emptyState.style.display = 'none';

  // Build cards HTML
  const cardsHtml = filtered.map(t => {
    const isVideo = getFileIcon(t.filename, t.url).includes('🎬');
    const isDownloading = t.status === 'DOWNLOADING';
    const isCompleted = t.status === 'COMPLETED';
    const isPaused = t.status === 'PAUSED';
    const isScheduled = t.status === 'SCHEDULED';

    const statusClass = isDownloading ? 'status-downloading' : (isCompleted ? 'status-completed' : (isScheduled ? 'status-scheduled' : 'status-paused'));

    // Render segment cells
    let segmentCellsHtml = '';
    if (t.segments && t.segments.length > 0) {
      segmentCellsHtml = t.segments.map(seg => {
        const segTotal = seg.end - seg.start + 1;
        const pct = segTotal > 0 ? Math.min(100, Math.round((seg.downloaded / segTotal) * 100)) : 0;
        const isDone = seg.status === 'COMPLETED' || pct >= 100;
        const isDown = seg.status === 'DOWNLOADING';
        const cellClass = isDone ? 'completed' : (isDown ? 'downloading' : '');
        return `<div class="segment-cell ${cellClass}"><div class="segment-cell-fill" style="width: ${pct}%"></div></div>`;
      }).join('');
    }

    return `
      <div class="task-card" data-id="${t.id}">
        <div class="task-header">
          <div class="task-title-group">
            <span class="file-icon">${getFileIcon(t.filename, t.url)}</span>
            <span class="task-filename" title="${t.filename}">${t.filename}</span>
            <span class="task-status-pill ${statusClass}">${t.status}</span>
          </div>
          <div class="task-actions">
            ${isVideo ? `
              <button class="btn btn-secondary btn-sm btn-watch" onclick="openPlayer('${t.id}', '${t.filename}')">
                🎬 Watch Now
              </button>
            ` : ''}
            ${isDownloading ? `
              <button class="btn btn-secondary btn-sm" onclick="pauseTask('${t.id}')">⏸ Pause</button>
            ` : (isPaused ? `
              <button class="btn btn-secondary btn-sm" onclick="resumeTask('${t.id}')">▶ Resume</button>
            ` : '')}
            <button class="btn btn-secondary btn-sm" onclick="openFolder('${t.id}')" title="Open Folder">📂</button>
            <button class="btn btn-secondary btn-sm" onclick="deleteTask('${t.id}')" title="Delete">🗑️</button>
          </div>
        </div>

        <!-- Progress Bar -->
        <div class="progress-track">
          <div class="progress-fill" style="width: ${t.progress_percent || 0}%"></div>
        </div>

        <!-- Segments Bar -->
        ${segmentCellsHtml ? `<div class="segments-visualizer" title="Multi-Thread Segments Progress">${segmentCellsHtml}</div>` : ''}

        <!-- Meta Footer -->
        <div class="task-meta">
          <div class="task-stats-left">
            <span>${formatBytes(t.downloaded_bytes)} / ${formatBytes(t.total_size)} (${(t.progress_percent || 0).toFixed(1)}%)</span>
            ${isDownloading ? `<span class="highlight-speed">⚡ ${formatBytes(t.speed_bytes_sec)}/s</span>` : ''}
            ${isDownloading && t.eta_seconds > 0 ? `<span>⏳ ETA: ${formatDuration(t.eta_seconds)}</span>` : ''}
          </div>
          <div>
            <span>Conns: <strong>${t.active_connections || (isCompleted ? 0 : 8)}</strong></span>
          </div>
        </div>
      </div>
    `;
  }).join('');

  taskListContainer.innerHTML = cardsHtml;
}

// Update Global Stats
function updateStats() {
  let totalSpeed = 0;
  let totalConns = 0;
  let activeCount = 0;
  let completedCount = 0;

  tasks.forEach(t => {
    if (t.status === 'DOWNLOADING') {
      totalSpeed += (t.speed_bytes_sec || 0);
      totalConns += (t.active_connections || 0);
      activeCount++;
    } else if (t.status === 'COMPLETED') {
      completedCount++;
    }
  });

  globalSpeedEl.textContent = `${(totalSpeed / (1024 * 1024)).toFixed(2)} MB/s`;
  globalConnsEl.textContent = totalConns;
  taskCountEl.textContent = `${activeCount} Active`;

  countAllEl.textContent = tasks.length;
  countActiveEl.textContent = activeCount;
  countCompletedEl.textContent = completedCount;

  // Add to chart history
  speedHistory.push(totalSpeed);
  speedHistory.shift();
  drawSpeedChart();
}

// Connect Server-Sent Events (SSE)
function initSSE() {
  const eventSource = new EventSource('/api/events');

  eventSource.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      if (Array.isArray(data)) {
        tasks = data;
        renderTasks();
        updateStats();
      }
    } catch (e) {
      console.error('Failed to parse SSE event:', e);
    }
  };

  eventSource.onerror = () => {
    console.warn('SSE connection disconnected. Reconnecting...');
  };
}

// REST API calls
async function fetchTasks() {
  try {
    const res = await fetch('/api/tasks');
    if (res.ok) {
      tasks = await res.json();
      renderTasks();
      updateStats();
    }
  } catch (e) {
    console.error('Fetch tasks error:', e);
  }
}

async function pauseTask(id) {
  await fetch(`/api/tasks/${id}/pause`, { method: 'POST' });
}

async function resumeTask(id) {
  await fetch(`/api/tasks/${id}/resume`, { method: 'POST' });
}

async function deleteTask(id) {
  if (confirm('Delete this download task from list?')) {
    await fetch(`/api/tasks/${id}`, { method: 'DELETE' });
    fetchTasks();
  }
}

async function openFolder(id) {
  await fetch(`/api/tasks/${id}/open-folder`, { method: 'POST' });
}

// Video Player Modal
window.openPlayer = function(id, filename) {
  const streamUrl = `${window.location.origin}/stream/${id}/${encodeURIComponent(filename)}`;
  playerVideoTitle.textContent = filename;
  playerStreamUrl.textContent = streamUrl;
  html5Player.src = streamUrl;
  videoPlayerModal.classList.add('open');
  html5Player.play().catch(() => {});
};

btnClosePlayer.onclick = () => {
  html5Player.pause();
  html5Player.src = '';
  videoPlayerModal.classList.remove('open');
};

btnCopyStreamUrl.onclick = () => {
  navigator.clipboard.writeText(playerStreamUrl.textContent);
  btnCopyStreamUrl.textContent = '✅ Copied!';
  setTimeout(() => btnCopyStreamUrl.textContent = '📋 Copy Stream URL', 1500);
};

// New Task Form
btnOpenNewTask.onclick = () => newTaskModal.classList.add('open');
btnCloseModal.onclick = () => newTaskModal.classList.remove('open');
btnCancelModal.onclick = () => newTaskModal.classList.remove('open');

inputConnections.oninput = (e) => {
  valConnections.textContent = e.target.value;
};

btnPasteUrl.onclick = async () => {
  try {
    const text = await navigator.clipboard.readText();
    if (text) inputUrl.value = text.trim();
  } catch (e) {}
};

const btnSelectTorrent = document.getElementById('btnSelectTorrent');
const inputTorrentFile = document.getElementById('inputTorrentFile');

if (btnSelectTorrent && inputTorrentFile) {
  btnSelectTorrent.onclick = () => inputTorrentFile.click();
  inputTorrentFile.onchange = async () => {
    if (!inputTorrentFile.files.length) return;
    const file = inputTorrentFile.files[0];
    const formData = new FormData();
    formData.append('torrent', file);
    const saveDir = document.getElementById('inputSaveDir').value.trim();
    if (saveDir) formData.append('target_path', saveDir);

    try {
      const res = await fetch('/api/tasks/upload-torrent', {
        method: 'POST',
        body: formData
      });
      if (res.ok) {
        newTaskModal.classList.remove('open');
        newTaskForm.reset();
        inputTorrentFile.value = '';
        fetchTasks();
      } else {
        const err = await res.text();
        alert('Failed to upload torrent: ' + err);
      }
    } catch (err) {
      alert('Failed to upload torrent: ' + err);
    }
  };
}

newTaskForm.onsubmit = async (e) => {
  e.preventDefault();
  const url = inputUrl.value.trim();
  const connections = parseInt(inputConnections.value, 10);
  const speedLimit = parseInt(document.getElementById('inputSpeedLimit').value, 10) || 0;
  const saveDir = document.getElementById('inputSaveDir').value.trim();
  const scheduleVal = document.getElementById('inputSchedule').value;

  try {
    const res = await fetch('/api/tasks', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        url: url,
        connections: connections,
        speed_limit_kb: speedLimit,
        target_path: saveDir,
        scheduled_at: scheduleVal || undefined
      })
    });
    if (res.ok) {
      newTaskModal.classList.remove('open');
      newTaskForm.reset();
      valConnections.textContent = '16';
      fetchTasks();
    }
  } catch (err) {
    alert('Failed to add task: ' + err);
  }
};

// Filter Navigation
document.querySelectorAll('.nav-item').forEach(btn => {
  btn.onclick = () => {
    document.querySelectorAll('.nav-item').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    currentFilter = btn.dataset.filter;
    renderTasks();
  };
});

searchInput.oninput = renderTasks;

// Initialize
drawSpeedChart();
fetchTasks();
initSSE();
