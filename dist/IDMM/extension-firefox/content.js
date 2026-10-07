// IDMM Video & Media Sniffer Content Script

function initMediaSniffer() {
  const processedVideos = new WeakSet();

  function attachSniffer(videoEl) {
    if (processedVideos.has(videoEl)) return;
    processedVideos.add(videoEl);

    // Create floating badge container
    const badge = document.createElement('div');
    badge.className = 'idmm-video-sniffer-btn';
    badge.innerHTML = `
      <span class="idmm-sniff-icon">⚡</span>
      <span class="idmm-sniff-text">Download with IDMM</span>
    `;

    // Position container wrapper if needed
    const wrapper = document.createElement('div');
    wrapper.className = 'idmm-video-wrapper';

    // Insert badge
    document.body.appendChild(badge);

    function updateBadgePosition() {
      const rect = videoEl.getBoundingClientRect();
      if (rect.width > 200 && rect.height > 100 && rect.top < window.innerHeight && rect.bottom > 0) {
        badge.style.display = 'flex';
        badge.style.top = `${Math.max(10, rect.top + window.scrollY + 10)}px`;
        badge.style.left = `${Math.max(10, rect.right + window.scrollX - 180)}px`;
      } else {
        badge.style.display = 'none';
      }
    }

    // Show badge on hover or play
    videoEl.addEventListener('mouseenter', updateBadgePosition);
    videoEl.addEventListener('play', updateBadgePosition);
    window.addEventListener('scroll', updateBadgePosition, { passive: true });
    window.addEventListener('resize', updateBadgePosition, { passive: true });

    badge.addEventListener('mouseenter', () => { badge.style.display = 'flex'; });
    videoEl.addEventListener('mouseleave', () => {
      setTimeout(() => {
        if (!badge.matches(':hover')) {
          badge.style.display = 'none';
        }
      }, 500);
    });

    badge.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();

      // Find best media source URL
      let mediaSrc = videoEl.currentSrc || videoEl.src;
      if (!mediaSrc) {
        const sourceEl = videoEl.querySelector('source');
        if (sourceEl) mediaSrc = sourceEl.src;
      }

      if (!mediaSrc || mediaSrc.startsWith('blob:')) {
        // Look for m3u8 or streaming links in page performance resource timings
        const entries = window.performance.getEntriesByType('resource');
        for (let i = entries.length - 1; i >= 0; i--) {
          const name = entries[i].name;
          if (name.includes('.m3u8') || name.includes('.mp4')) {
            mediaSrc = name;
            break;
          }
        }
      }

      if (mediaSrc) {
        badge.innerHTML = `<span class="idmm-sniff-icon">⏳</span><span>Dispatching...</span>`;
        chrome.runtime.sendMessage({
          action: "download_media",
          url: mediaSrc,
          filename: document.title.replace(/[^a-zA-Z0-9_-]/g, '_') + '.mp4'
        }, () => {
          badge.innerHTML = `<span class="idmm-sniff-icon">✅</span><span>Sent to IDMM!</span>`;
          setTimeout(() => {
            badge.innerHTML = `<span class="idmm-sniff-icon">⚡</span><span>Download with IDMM</span>`;
          }, 2000);
        });
      } else {
        alert("IDMM: Media stream URL not directly accessible.");
      }
    });
  }

  // Find all current video elements
  document.querySelectorAll('video').forEach(attachSniffer);

  // Observe DOM for dynamically loaded video players (YouTube, Facebook, etc.)
  const observer = new MutationObserver((mutations) => {
    for (const mutation of mutations) {
      for (const node of mutation.addedNodes) {
        if (node.nodeType === Node.ELEMENT_NODE) {
          if (node.tagName === 'VIDEO') {
            attachSniffer(node);
          } else {
            node.querySelectorAll('video').forEach(attachSniffer);
          }
        }
      }
    }
  });

  observer.observe(document.body, { childList: true, subtree: true });
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', initMediaSniffer);
} else {
  initMediaSniffer();
}
