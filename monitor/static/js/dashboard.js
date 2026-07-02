let map;
let markers = {};
let nodes = [];
let hubs = [];
let hubIds = new Set();
let trustByNode = {};
let orphanHints = [];
let orphanLayers = [];
let routes = [];
let infrastructure = [];
let infraMapMarkers = {};
let graphNetwork = null;
let hopChart = null;
let fallbackEnabled = false;

function showView(name) {
  for (const el of document.querySelectorAll('main > section')) el.classList.add('hidden');
  document.getElementById('view-' + name)?.classList.remove('hidden');
  for (const a of document.querySelectorAll('nav a[data-view]')) {
    a.classList.toggle('active', a.dataset.view === name);
  }
  if (name === 'map') ensureMap();
  if (name === 'graph') ensureGraph();
  if (name === 'hop-timing') loadHopLatency();
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
  document.getElementById('stat-hubs-total').textContent = o.total_hubs ?? o.TotalHubs ?? '0';
  document.getElementById('stat-hubs-online').textContent = o.online_hubs ?? o.OnlineHubs ?? '0';
  document.getElementById('stat-hubs-offline').textContent = o.offline_hubs ?? o.OfflineHubs ?? '0';
  document.getElementById('stat-routes').textContent = o.route_count ?? o.RouteCount ?? '0';
  document.getElementById('stat-dtn').textContent = o.dtn_depth ?? o.DTNDepth ?? '0';
  const fb = o.internet_fallback_enabled ?? o.InternetFallback;
  if (fb != null) setFallbackUI(!!fb);
}

function setFallbackUI(enabled) {
  fallbackEnabled = enabled;
  const el = document.getElementById('stat-fallback');
  if (el) el.textContent = enabled ? 'allowed' : 'off';
  const toggle = document.getElementById('fallback-toggle');
  if (toggle) toggle.checked = enabled;
}

function hubEndpoint(h) {
  if (h.last_ip && h.last_port) return `${h.last_ip}:${h.last_port}`;
  return '—';
}

function trustColor(score) {
  const s = score ?? 0;
  if (s >= 76) return '#6cb6ff';
  if (s >= 51) return '#3fb950';
  if (s >= 26) return '#d29922';
  return '#da3633';
}

function trustClass(score) {
  const s = score ?? 0;
  if (s >= 76) return 'trust-blue';
  if (s >= 51) return 'trust-green';
  if (s >= 26) return 'trust-amber';
  return 'trust-red';
}

function renderHubsTable(tbodyId, detailed) {
  const tbody = document.getElementById(tbodyId);
  if (!tbody) return;
  tbody.innerHTML = '';
  for (const h of hubs) {
    const tr = document.createElement('tr');
    const cls = h.status === 'online' ? 'status-online' : 'status-stale';
    if (detailed) {
      tr.innerHTML = `<td class="kind-hub" title="${h.hub_id}">${shortId(h.hub_id)}</td>
        <td>${hubEndpoint(h)}</td>
        <td>${h.relay_capable ? 'yes' : 'no'}</td>
        <td class="${cls}">${h.status}</td>
        <td>${formatDateTime(h.first_seen)}</td>
        <td>${formatDateTime(h.last_seen)}</td>`;
    } else {
      tr.innerHTML = `<td class="kind-hub" title="${h.hub_id}">${shortId(h.hub_id)}</td>
        <td>${hubEndpoint(h)}</td>
        <td class="${cls}">${h.status}</td>
        <td>${formatDateTime(h.last_seen)}</td>`;
    }
    tbody.appendChild(tr);
  }
}

function renderNodesTable() {
  const tbody = document.getElementById('nodes-table');
  tbody.innerHTML = '';
  for (const n of nodes) {
    const tr = document.createElement('tr');
    const cls = n.status === 'online' ? 'status-online' : 'status-stale';
    const score = trustByNode[n.node_id]?.total;
    const trustLabel = score != null ? ` · trust ${score}` : '';
    tr.innerHTML = `<td title="${n.node_id}">${shortId(n.node_id)}</td>
      <td class="${cls}">${n.status}${trustLabel}</td>
      <td>${formatDateTime(n.last_seen)}</td>`;
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
    const score = trustByNode[n.node_id]?.total ?? 0;
    if (n.lat != null && n.lon != null) {
      withGps.push(n);
      if (!markers[n.node_id]) {
        markers[n.node_id] = L.circleMarker([n.lat, n.lon], {
          radius: 9,
          fillColor: trustColor(score),
          color: '#e7ecf1',
          weight: 1,
          fillOpacity: 0.85,
        }).addTo(map)
          .bindPopup(`<strong>${shortId(n.node_id)}</strong><br>${n.status}<br>Trust: ${score}`);
      } else {
        markers[n.node_id].setLatLng([n.lat, n.lon]);
        markers[n.node_id].setStyle({ fillColor: trustColor(score) });
      }
    } else {
      const li = document.createElement('li');
      li.innerHTML = `${shortId(n.node_id)} — ${n.status} — <span class="${trustClass(score)}">trust ${score}</span>`;
      noGps.appendChild(li);
    }
  }
  if (withGps.length) {
    const bounds = L.latLngBounds(withGps.map((n) => [n.lat, n.lon]));
    map.fitBounds(bounds.pad(0.2));
  }
  refreshOrphanMap();
  refreshInfrastructureMap();
}

function formatBearing(deg) {
  const dirs = ['N', 'NE', 'E', 'SE', 'S', 'SW', 'W', 'NW'];
  const idx = Math.round(deg / 45) % 8;
  return `${deg.toFixed(0)}° ${dirs[idx]}`;
}

function formatDistance(m) {
  if (m >= 1000) return `${(m / 1000).toFixed(1)} km`;
  return `${Math.round(m)} m`;
}

function renderOrphanList() {
  const list = document.getElementById('orphan-hints-list');
  if (!list) return;
  list.innerHTML = '';
  if (!orphanHints.length) {
    const li = document.createElement('li');
    li.textContent = 'No stale nodes — nothing to recover.';
    list.appendChild(li);
    return;
  }
  for (const h of orphanHints) {
    const li = document.createElement('li');
    const bearing = h.distance_m > 0 ? formatBearing(h.bearing_deg) : '—';
    const dist = h.distance_m > 0 ? formatDistance(h.distance_m) : '—';
    let detail = `${formatDateTime(h.last_seen)} · ${h.confidence} · ${h.age_label}`;
    if (h.proximity_note) detail += ` · ${h.proximity_note}`;
    li.innerHTML = `<strong>${shortId(h.node_id)}</strong> — ${bearing} / ${dist}<br>`
      + `<span class="status-stale">${detail}</span>`;
    list.appendChild(li);
  }
}

function refreshOrphanMap() {
  if (!map) return;
  for (const layer of orphanLayers) map.removeLayer(layer);
  orphanLayers = [];
  const hasRef = orphanHints.some((h) => h.ref_lat !== 0 || h.ref_lon !== 0);
  let refDrawn = false;
  for (const h of orphanHints) {
    if (h.last_lat == null || h.last_lon == null) continue;
    const popup = `<strong>${shortId(h.node_id)}</strong> (stale)<br>`
      + `${formatBearing(h.bearing_deg)} · ${formatDistance(h.distance_m)}<br>`
      + `${h.confidence} · ${h.age_label}`;
    const marker = L.circleMarker([h.last_lat, h.last_lon], {
      radius: 11,
      color: '#d29922',
      fillColor: '#8b6914',
      fillOpacity: 0.45,
      weight: 2,
      dashArray: '4 4',
    }).addTo(map).bindPopup(popup);
    orphanLayers.push(marker);
    if (hasRef && h.distance_m > 0) {
      const line = L.polyline(
        [[h.ref_lat, h.ref_lon], [h.last_lat, h.last_lon]],
        { color: '#d29922', weight: 2, dashArray: '8 6', opacity: 0.75 },
      ).addTo(map);
      orphanLayers.push(line);
      if (!refDrawn) {
        const refMarker = L.circleMarker([h.ref_lat, h.ref_lon], {
          radius: 5,
          color: '#6cb6ff',
          fillColor: '#1f6feb',
          fillOpacity: 0.8,
          weight: 1,
        }).addTo(map).bindPopup('Reference (online nodes centroid)');
        orphanLayers.push(refMarker);
        refDrawn = true;
      }
    }
  }
}

async function loadOrphanHints() {
  orphanHints = await api('/api/orphan-hints') || [];
  renderOrphanList();
  refreshOrphanMap();
  refreshInfrastructureMap();
}

async function loadNodes() {
  nodes = await api('/api/nodes') || [];
  renderNodesTable();
  refreshMapMarkers();
  refreshGraph();
}

async function loadHubs() {
  hubs = await api('/api/hubs') || [];
  hubIds = new Set(hubs.map((h) => h.hub_id));
  renderHubsTable('hubs-table-overview', false);
  renderHubsTable('hubs-table', true);
  renderMapHubList();
  refreshGraph();
}

function renderMapHubList() {
  const list = document.getElementById('map-hubs-list');
  if (!list) return;
  list.innerHTML = '';
  for (const h of hubs) {
    const li = document.createElement('li');
    const cls = h.status === 'online' ? 'status-online' : 'status-stale';
    li.innerHTML = `<span class="kind-hub">${shortId(h.hub_id)}</span> — ${hubEndpoint(h)} — `
      + `<span class="${cls}">${h.status}</span> — ${formatDateTime(h.last_seen)}`;
    list.appendChild(li);
  }
}

async function loadTrustScores() {
  const scores = await api('/api/trust-scores') || [];
  trustByNode = {};
  for (const s of scores) {
    trustByNode[s.node_id] = s;
  }
  renderTrustTable();
  renderNodesTable();
  refreshMapMarkers();
}

function renderTrustTable() {
  const tbody = document.getElementById('trust-table');
  if (!tbody) return;
  tbody.innerHTML = '';
  const rows = Object.values(trustByNode).sort((a, b) => (b.total ?? 0) - (a.total ?? 0));
  for (const s of rows) {
    const tr = document.createElement('tr');
    const cls = s.status === 'online' ? 'status-online' : 'status-stale';
    tr.innerHTML = `<td title="${s.node_id}">${shortId(s.node_id)}</td>
      <td class="${cls}">${s.status}</td>
      <td class="${trustClass(s.total)}">${s.total}</td>
      <td>${s.longevity}</td>
      <td>${s.proximity}</td>
      <td>${s.endorsements}</td>
      <td>${s.proximity_events}</td>
      <td>${s.endorsement_count}</td>`;
    tbody.appendChild(tr);
  }
}

async function loadRoutes() {
  routes = await api('/api/routes') || [];
  renderRoutesTable();
  refreshGraph();
}

function renderRoutesTable() {
  const tbody = document.getElementById('routes-table');
  if (!tbody) return;
  tbody.innerHTML = '';
  for (const r of routes) {
    const tr = document.createElement('tr');
    tr.innerHTML = `<td title="${r.destination}">${shortId(r.destination)}</td>
      <td title="${r.next_hop}">${shortId(r.next_hop)}</td>
      <td>${r.hop_count}</td>
      <td>${(r.tq ?? 0).toFixed(2)}</td>
      <td>${r.latency_ms ?? 0}</td>`;
    tbody.appendChild(tr);
  }
}

function infraGraphID(segmentID) {
  return `lan:${segmentID}`;
}

function infraLabel(seg) {
  if (seg.ssid) return seg.ssid;
  return seg.gateway_ip;
}

function infraGraphNode(seg) {
  const label = infraLabel(seg);
  const members = (seg.member_count ?? 0) || ((seg.node_ids?.length ?? 0) + (seg.hub_ids?.length ?? 0));
  return {
    id: infraGraphID(seg.segment_id),
    label,
    shape: 'hexagon',
    size: 22,
    font: { color: '#e7ecf1', size: 12 },
    color: {
      background: '#8957e5',
      border: '#a371f7',
      highlight: { background: '#a371f7', border: '#d2a8ff' },
    },
    title: `LAN segment\nGateway: ${seg.gateway_ip}\nSSID: ${seg.ssid || '—'}\nMembers: ${members}`,
    group: 'infrastructure',
  };
}

function refreshInfrastructureMap() {
  if (!map) return;
  const active = new Set();
  for (const seg of infrastructure) {
    if (seg.estimated_lat == null || seg.estimated_lon == null) continue;
    active.add(seg.segment_id);
    const members = (seg.member_count ?? 0) || ((seg.node_ids?.length ?? 0) + (seg.hub_ids?.length ?? 0));
    const popup = `<strong>${seg.ssid || 'LAN segment'}</strong><br>`
      + `Gateway: ${seg.gateway_ip}<br>`
      + `Members: ${members}<br>`
      + `Last seen: ${formatDateTime(seg.last_seen)}`;
    if (!infraMapMarkers[seg.segment_id]) {
      infraMapMarkers[seg.segment_id] = L.circleMarker([seg.estimated_lat, seg.estimated_lon], {
        radius: 13,
        color: '#a371f7',
        fillColor: '#8957e5',
        fillOpacity: 0.75,
        weight: 2,
        dashArray: '6 4',
      }).addTo(map).bindPopup(popup);
    } else {
      infraMapMarkers[seg.segment_id].setLatLng([seg.estimated_lat, seg.estimated_lon]);
      infraMapMarkers[seg.segment_id].setPopupContent(popup);
    }
  }
  for (const id of Object.keys(infraMapMarkers)) {
    if (!active.has(id)) {
      map.removeLayer(infraMapMarkers[id]);
      delete infraMapMarkers[id];
    }
  }
}

async function loadInfrastructure() {
  infrastructure = await api('/api/infrastructure') || [];
  refreshInfrastructureMap();
  refreshGraph();
}

function hubGraphNode(h) {
  const online = h.status === 'online';
  return {
    id: h.hub_id,
    label: shortId(h.hub_id),
    shape: 'diamond',
    size: 24,
    font: { color: '#e7ecf1', size: 13 },
    color: online
      ? { background: '#1f6feb', border: '#6cb6ff', highlight: { background: '#388bfd', border: '#6cb6ff' } }
      : { background: '#3d4f7a', border: '#6e7681', highlight: { background: '#4d5f8a', border: '#8b949e' } },
    title: `hub · ${h.status}\n${hubEndpoint(h)}\nlast seen: ${formatDateTime(h.last_seen)}`,
    group: 'hub',
  };
}

function graphNodeForId(id) {
  if (hubIds.has(id)) {
    const h = hubs.find((x) => x.hub_id === id);
    if (h) return hubGraphNode(h);
    return {
      id,
      label: shortId(id),
      shape: 'diamond',
      size: 24,
      color: { background: '#1f6feb', border: '#6cb6ff' },
      title: 'hub',
      group: 'hub',
    };
  }
  const n = nodes.find((x) => x.node_id === id);
  if (n) {
    const score = trustByNode[n.node_id]?.total ?? 0;
    const fill = trustColor(score);
    return {
      id: n.node_id,
      label: shortId(n.node_id),
      shape: 'dot',
      size: 16,
      color: { background: fill, border: '#e7ecf1', highlight: { background: fill, border: '#fff' } },
      title: `node · ${n.status} · trust ${score}`,
      group: 'node',
    };
  }
  return {
    id,
    label: shortId(id),
    shape: 'dot',
    size: 14,
    color: { background: '#6e7681', border: '#9da7b3' },
    group: 'node',
  };
}

function ensureGraph() {
  if (graphNetwork) {
    refreshGraph();
    return;
  }
  const container = document.getElementById('network-graph');
  if (!container || typeof vis === 'undefined') return;
  graphNetwork = new vis.Network(container, { nodes: [], edges: [] }, {
    physics: { stabilization: true },
    nodes: { font: { color: '#e7ecf1' } },
    edges: { color: { color: '#6cb6ff' }, arrows: 'to' },
    groups: {
      hub: { shape: 'diamond' },
      node: { shape: 'dot' },
      infrastructure: { shape: 'hexagon' },
    },
  });
  refreshGraph();
}

function refreshGraph() {
  if (!graphNetwork) return;
  const nodeMap = new Map();

  for (const h of hubs) {
    nodeMap.set(h.hub_id, hubGraphNode(h));
  }
  for (const n of nodes) {
    if (hubIds.has(n.node_id)) continue;
    nodeMap.set(n.node_id, graphNodeForId(n.node_id));
  }
  for (const r of routes) {
    if (!nodeMap.has(r.destination)) {
      nodeMap.set(r.destination, graphNodeForId(r.destination));
    }
    if (!nodeMap.has(r.next_hop)) {
      nodeMap.set(r.next_hop, graphNodeForId(r.next_hop));
    }
  }
  for (const seg of infrastructure) {
    nodeMap.set(infraGraphID(seg.segment_id), infraGraphNode(seg));
  }

  const edgeMap = new Map();
  for (const r of routes) {
    const key = `${r.next_hop}->${r.destination}`;
    const bothHubs = hubIds.has(r.next_hop) && hubIds.has(r.destination);
    edgeMap.set(key, {
      id: key,
      from: r.next_hop,
      to: r.destination,
      label: bothHubs ? 'hub' : `tq ${(r.tq ?? 0).toFixed(2)}`,
      title: bothHubs
        ? 'hub route'
        : `route · hops: ${r.hop_count}, latency: ${r.latency_ms}ms`,
      arrows: 'to',
      width: bothHubs ? 2.5 : 1,
      color: bothHubs ? { color: '#6cb6ff' } : { color: '#58a6ff' },
    });
  }
  for (const seg of infrastructure) {
    const segID = infraGraphID(seg.segment_id);
    for (const nodeID of seg.node_ids || []) {
      if (!nodeMap.has(nodeID)) {
        nodeMap.set(nodeID, graphNodeForId(nodeID));
      }
      const key = `${nodeID}->${segID}`;
      edgeMap.set(key, {
        id: key,
        from: nodeID,
        to: segID,
        dashes: [8, 6],
        width: 1.5,
        color: { color: '#a371f7' },
        title: 'LAN segment membership',
      });
    }
    for (const hubID of seg.hub_ids || []) {
      if (!nodeMap.has(hubID)) {
        nodeMap.set(hubID, graphNodeForId(hubID));
      }
      const key = `${hubID}->${segID}`;
      edgeMap.set(key, {
        id: key,
        from: hubID,
        to: segID,
        dashes: [8, 6],
        width: 1.5,
        color: { color: '#a371f7' },
        title: 'LAN segment membership',
      });
    }
  }

  graphNetwork.setData({
    nodes: new vis.DataSet([...nodeMap.values()]),
    edges: new vis.DataSet([...edgeMap.values()]),
  });
}

async function loadHopLatency() {
  const points = await api('/api/metrics/hop-latency?window=1h') || [];
  renderHopChart(points);
}

function renderHopChart(points) {
  const canvas = document.getElementById('hop-latency-chart');
  if (!canvas || typeof Chart === 'undefined') return;
  const sorted = [...points].sort((a, b) => new Date(a.recorded_at) - new Date(b.recorded_at));
  const labels = sorted.map((p) => formatDateTime(p.recorded_at));
  const values = sorted.map((p) => p.value);
  if (hopChart) {
    hopChart.data.labels = labels;
    hopChart.data.datasets[0].data = values;
    hopChart.update();
    return;
  }
  hopChart = new Chart(canvas, {
    type: 'line',
    data: {
      labels,
      datasets: [{
        label: 'Route latency (ms)',
        data: values,
        borderColor: '#6cb6ff',
        backgroundColor: 'rgba(108, 182, 255, 0.15)',
        fill: true,
        tension: 0.2,
        pointRadius: sorted.length > 80 ? 0 : 3,
      }],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      scales: {
        x: { ticks: { color: '#9da7b3', maxTicksLimit: 8 } },
        y: { ticks: { color: '#9da7b3' }, title: { display: true, text: 'ms', color: '#9da7b3' } },
      },
      plugins: { legend: { labels: { color: '#e7ecf1' } } },
    },
  });
}

async function loadInternetFallback() {
  const cfg = await api('/api/internet-fallback');
  if (cfg) setFallbackUI(!!cfg.enabled);
}

async function saveInternetFallback(enabled) {
  const cfg = await api('/api/internet-fallback', {
    method: 'PATCH',
    body: JSON.stringify({ enabled }),
  });
  if (cfg) setFallbackUI(!!cfg.enabled);
}

document.getElementById('fallback-toggle')?.addEventListener('change', async (e) => {
  try {
    await saveInternetFallback(e.target.checked);
  } catch (_) {
    e.target.checked = fallbackEnabled;
  }
});

async function loadAppStats() {
  const stats = await api('/api/app-stats') || [];
  const tbody = document.getElementById('app-stats-table');
  if (!tbody) return;
  tbody.innerHTML = '';
  if (!stats.length) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="5">No registered apps yet.</td>';
    tbody.appendChild(tr);
    return;
  }
  for (const s of stats) {
    const tr = document.createElement('tr');
    tr.innerHTML = `<td>${s.app_id}</td><td>${s.app_version}</td><td>${s.node_count}</td>
      <td>${formatDateTime(s.first_seen)}</td><td>${formatDateTime(s.last_seen)}</td>`;
    tbody.appendChild(tr);
  }
}

async function loadSosAlerts() {
  const rows = await api('/api/sos-alerts') || [];
  const tbody = document.getElementById('sos-table');
  if (!tbody) return;
  tbody.innerHTML = '';
  if (!rows.length) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="4">No SOS alerts yet. Publish with <code>go run ./apps/sosbeacon</code>.</td>';
    tbody.appendChild(tr);
    return;
  }
  for (const a of rows) {
    const tr = document.createElement('tr');
    const loc = (a.lat || a.lon)
      ? `${a.lat?.toFixed?.(5) ?? a.lat}, ${a.lon?.toFixed?.(5) ?? a.lon}${a.accuracy_m ? ` (±${Math.round(a.accuracy_m)} m)` : ''}`
      : '—';
    const text = a.text || a.raw_payload || '—';
    tr.innerHTML = `<td>${formatDateTime(a.received_at)}</td><td>${shortId(a.node_id)}</td>
      <td>${text}</td><td>${loc}</td>`;
    tbody.appendChild(tr);
  }
}

async function loadBanList() {
  const rows = await api('/api/ban-list') || [];
  const tbody = document.getElementById('ban-table');
  if (!tbody) return;
  tbody.innerHTML = '';
  if (!rows.length) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="7">No ban proposals.</td>';
    tbody.appendChild(tr);
    return;
  }
  for (const b of rows) {
    const tr = document.createElement('tr');
    const local = b.locally_enforced ? 'enforced' : (b.local_verdict || 'pending');
    const cls = b.locally_enforced ? 'status-stale' : (b.local_verdict === 'agree' ? 'status-online' : '');
    tr.innerHTML = `<td>${b.app_id}</td><td>${b.version_range}</td><td>${b.reason}</td>
      <td>${b.agree_count}</td><td>${b.disagree_count}</td>
      <td class="${cls}">${local}</td>
      <td>
        <button class="secondary ban-agree" data-id="${b.ban_id}">Agree</button>
        <button class="danger ban-disagree" data-id="${b.ban_id}">Disagree</button>
      </td>`;
    tbody.appendChild(tr);
  }
  tbody.querySelectorAll('.ban-agree').forEach((btn) => {
    btn.addEventListener('click', () => setBanVerdict(btn.dataset.id, true));
  });
  tbody.querySelectorAll('.ban-disagree').forEach((btn) => {
    btn.addEventListener('click', () => setBanVerdict(btn.dataset.id, false));
  });
}

async function setBanVerdict(id, agree) {
  await api('/api/ban-list/' + id + '/verdict', {
    method: 'POST',
    body: JSON.stringify({ agree }),
  });
  await loadBanList();
}

document.getElementById('propose-ban')?.addEventListener('submit', async (e) => {
  e.preventDefault();
  await api('/api/ban-list', {
    method: 'POST',
    body: JSON.stringify({
      app_id: document.getElementById('ban-app-id').value.trim(),
      version_range: document.getElementById('ban-version').value.trim() || '*',
      reason: document.getElementById('ban-reason').value.trim(),
    }),
  });
  document.getElementById('ban-app-id').value = '';
  document.getElementById('ban-version').value = '';
  document.getElementById('ban-reason').value = '';
  await loadBanList();
});

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
    tr.innerHTML = `<td title="${h.hub_id}">${shortId(h.hub_id)}</td><td>${h.ip}</td><td>${h.port}</td><td>${h.source}</td>
      <td>${formatDateTime(h.last_verified)}</td>
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
    loadTrustScores();
    loadOrphanHints();
    loadInfrastructure();
    loadOverview();
  }
  if (ev.type === 'hub_status_changed') {
    loadHubs();
    loadRoutes();
    loadInfrastructure();
    loadOverview();
  }
  if (ev.type === 'route_changed') {
    loadOverview();
    loadRoutes();
    loadHubs();
    loadHopLatency();
  }
  if (ev.type === 'dtn_queue_depth_changed') {
    document.getElementById('stat-dtn').textContent = ev.dtn_depth;
  }
  if (ev.type === 'internet_fallback_changed') {
    setFallbackUI(!!ev.enabled);
  }
  if (ev.type === 'app_presence_changed') {
    loadAppStats();
  }
  if (ev.type === 'sos_alert_published') {
    loadSosAlerts();
  }
  if (ev.type === 'ban_proposal_changed' || ev.type === 'ban_verdict_changed') {
    loadBanList();
  }
});

(async function init() {
  try {
    await loadOverview();
    await loadHubs();
    await loadNodes();
    await loadTrustScores();
    await loadOrphanHints();
    await loadRoutes();
    await loadInfrastructure();
    await loadRelayHubs();
    await loadInternetFallback();
    await loadAppStats();
    await loadSosAlerts();
    await loadBanList();
  } catch (_) {
    window.location.href = '/login';
  }
})();
