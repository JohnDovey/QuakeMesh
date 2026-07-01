let map;
let markers = {};
let nodes = [];

function showView(name) {
  for (const el of document.querySelectorAll('main > section')) el.classList.add('hidden');
  document.getElementById('view-' + name)?.classList.remove('hidden');
  for (const a of document.querySelectorAll('nav a[data-view]')) {
    a.classList.toggle('active', a.dataset.view === name);
  }
  if (name === 'map') ensureMap();
}

document.querySelectorAll('nav a[data-view]').forEach((a) => {
  a.addEventListener('click', (e) => {
    e.preventDefault();
    showView(a.dataset.view);
  });
});

function applyOverview(o) {
  document.getElementById('stat-total').textContent = o.total_nodes ?? o.TotalNodes ?? '0';
  document.getElementById('stat-online').textContent = o.online_nodes ?? o.OnlineNodes ?? '0';
  document.getElementById('stat-offline').textContent = o.offline_nodes ?? o.OfflineNodes ?? '0';
  document.getElementById('stat-routes').textContent = o.route_count ?? o.RouteCount ?? '0';
  document.getElementById('stat-dtn').textContent = o.dtn_depth ?? o.DTNDepth ?? '0';
}

function renderNodesTable() {
  const tbody = document.getElementById('nodes-table');
  tbody.innerHTML = '';
  for (const n of nodes) {
    const tr = document.createElement('tr');
    const cls = n.status === 'online' ? 'status-online' : 'status-stale';
    tr.innerHTML = `<td title="${n.node_id}">${shortId(n.node_id)}</td>
      <td class="${cls}">${n.status}</td>
      <td>${new Date(n.last_seen).toLocaleString()}</td>`;
    tbody.appendChild(tr);
  }
}

function ensureMap() {
  if (map) {
    refreshMapMarkers();
    return;
  }
  map = L.map('map').setView([0, 0], 2);
  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    attribution: '&copy; OpenStreetMap contributors',
    maxZoom: 19,
  }).addTo(map);
  refreshMapMarkers();
}

function refreshMapMarkers() {
  if (!map) return;
  const noGps = document.getElementById('no-gps-list');
  noGps.innerHTML = '';
  const withGps = [];
  for (const n of nodes) {
    if (n.lat != null && n.lon != null) {
      withGps.push(n);
      if (!markers[n.node_id]) {
        markers[n.node_id] = L.marker([n.lat, n.lon]).addTo(map)
          .bindPopup(`<strong>${shortId(n.node_id)}</strong><br>${n.status}`);
      } else {
        markers[n.node_id].setLatLng([n.lat, n.lon]);
      }
    } else {
      const li = document.createElement('li');
      li.textContent = `${shortId(n.node_id)} — ${n.status}`;
      noGps.appendChild(li);
    }
  }
  if (withGps.length) {
    const bounds = L.latLngBounds(withGps.map((n) => [n.lat, n.lon]));
    map.fitBounds(bounds.pad(0.2));
  }
}

async function loadNodes() {
  nodes = await api('/api/nodes') || [];
  renderNodesTable();
  refreshMapMarkers();
}

async function loadOverview() {
  const o = await api('/api/overview');
  if (o) applyOverview(o);
}

async function loadRelayHubs() {
  const hubs = await api('/api/relay-hubs') || [];
  const tbody = document.getElementById('relay-table');
  tbody.innerHTML = '';
  for (const h of hubs) {
    const tr = document.createElement('tr');
    tr.innerHTML = `<td>${h.ip}</td><td>${h.port}</td><td>${h.source}</td>
      <td>${new Date(h.last_verified).toLocaleString()}</td>
      <td>
        <button class="secondary probe-btn" data-id="${h.hub_id}">Probe</button>
        <button class="danger delete-btn" data-id="${h.hub_id}">Remove</button>
      </td>`;
    tbody.appendChild(tr);
  }
  tbody.querySelectorAll('.probe-btn').forEach((btn) => {
    btn.addEventListener('click', () => probeRelay(btn.dataset.id));
  });
  tbody.querySelectorAll('.delete-btn').forEach((btn) => {
    btn.addEventListener('click', () => deleteRelay(btn.dataset.id));
  });
}

async function probeRelay(id) {
  await api('/api/relay-hubs/' + id + '/probe', { method: 'POST' });
  await loadRelayHubs();
}

async function deleteRelay(id) {
  await api('/api/relay-hubs/' + id, { method: 'DELETE' });
  await loadRelayHubs();
}

document.getElementById('add-relay').addEventListener('submit', async (e) => {
  e.preventDefault();
  const ip = document.getElementById('relay-ip').value.trim();
  const port = parseInt(document.getElementById('relay-port').value, 10);
  const hub = await api('/api/relay-hubs', {
    method: 'POST',
    body: JSON.stringify({ ip, port }),
  });
  if (hub) await probeRelay(hub.hub_id);
  document.getElementById('relay-ip').value = '';
  document.getElementById('relay-port').value = '';
  await loadRelayHubs();
});

connectWS((ev) => {
  if (ev.type === 'overview_snapshot' || ev.type === 'overview') {
    applyOverview(ev);
  }
  if (ev.type === 'node_status_changed') {
    loadNodes();
    loadOverview();
  }
  if (ev.type === 'route_changed') {
    loadOverview();
  }
  if (ev.type === 'dtn_queue_depth_changed') {
    document.getElementById('stat-dtn').textContent = ev.dtn_depth;
  }
});

(async function init() {
  try {
    await loadOverview();
    await loadNodes();
    await loadRelayHubs();
  } catch (_) {
    window.location.href = '/login';
  }
})();
