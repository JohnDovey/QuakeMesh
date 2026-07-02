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

const dateTimeFmt = new Intl.DateTimeFormat(undefined, {
  year: 'numeric',
  month: 'short',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  timeZoneName: 'short',
});

function formatDateTime(value) {
  const d = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(d.getTime())) return '—';
  return dateTimeFmt.format(d);
}

function collapseMobileNav() {
  const el = document.getElementById('mainNav');
  if (!el || !el.classList.contains('show')) return;
  if (typeof bootstrap === 'undefined') {
    el.classList.remove('show');
    return;
  }
  const inst = bootstrap.Collapse.getInstance(el) || new bootstrap.Collapse(el, { toggle: false });
  inst.hide();
}

document.addEventListener('DOMContentLoaded', () => {
  const logout = document.getElementById('logout');
  if (!logout) return;
  logout.addEventListener('click', async (e) => {
    e.preventDefault();
    await api('/api/logout', { method: 'POST' });
    window.location.href = '/login';
  });
});
