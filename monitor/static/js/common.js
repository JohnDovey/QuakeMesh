async function api(path, options = {}) {
  const res = await fetch(path, {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  });
  if (res.status === 401) {
    window.location.href = '/login';
    return null;
  }
  if (res.status === 403) {
    window.location.href = '/change-password';
    return null;
  }
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || res.statusText);
  }
  const ct = res.headers.get('content-type') || '';
  if (ct.includes('application/json')) return res.json();
  return null;
}

function connectWS(onMessage) {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const ws = new WebSocket(proto + '//' + location.host + '/ws');
  ws.onmessage = (ev) => {
    try { onMessage(JSON.parse(ev.data)); } catch (_) {}
  };
  ws.onclose = () => setTimeout(() => connectWS(onMessage), 2000);
  return ws;
}

function shortId(id) {
  if (!id || id.length < 12) return id || '';
  return id.slice(0, 8) + '…';
}

document.getElementById('logout')?.addEventListener('click', async (e) => {
  e.preventDefault();
  await api('/api/logout', { method: 'POST' });
  window.location.href = '/login';
});
