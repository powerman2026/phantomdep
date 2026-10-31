// PhantomDep UI — no framework, no build step.
// Every number rendered here comes from /api/scan; nothing is recomputed in the
// browser except the layout, which is presentation only (NFR4).

const $ = (sel) => document.querySelector(sel);

const el = {
  health: $('#health'),
  fixtures: $('#fixtures'),
  form: $('#scan-form'),
  path: $('#path'),
  scan: $('#scan'),
  banners: $('#banners'),
  stats: $('#stats'),
  graph: $('#graph'),
  detail: $('#detail'),
  findings: $('#findings'),
  findingCount: $('#finding-count'),
  packages: $('#packages'),
  packageCount: $('#package-count'),
  timing: $('#timing'),
};

let state = { result: null, selected: null };

const esc = (s) =>
  String(s ?? '').replace(/[&<>"']/g, (c) =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])
  );

// ------------------------------------------------------------------ bootstrap

async function loadHealth() {
  try {
    const res = await fetch('/api/health');
    const data = await res.json();
    el.health.innerHTML = data.stages
      .map(
        (s) =>
          `<span class="stage ${esc(s.status)}" title="${esc(s.detail || '')}">${esc(
            s.name
          )}: ${esc(s.status)}</span>`
      )
      .join('');
  } catch {
    el.health.innerHTML = '<span class="stage degraded">pipeline unreachable</span>';
  }
}

async function loadFixtures() {
  try {
    const res = await fetch('/api/fixtures');
    const { fixtures } = await res.json();
    el.fixtures.innerHTML = fixtures
      .map(
        (f) =>
          `<button type="button" class="chip" data-path="${esc(f.path)}" title="${esc(
            f.note || ''
          )}">${esc(f.name)}</button>`
      )
      .join('');
    el.fixtures.querySelectorAll('.chip').forEach((chip) => {
      chip.addEventListener('click', () => {
        el.path.value = chip.dataset.path;
        runScan();
      });
    });
  } catch {
    /* fixtures are a convenience; failure is not fatal */
  }
}

// ----------------------------------------------------------------------- scan

async function runScan() {
  const path = el.path.value.trim();
  if (!path) {
    banner('degraded', 'scan', 'Enter a repository path or pick a fixture.');
    return;
  }
  el.scan.disabled = true;
  el.scan.textContent = 'Scanning…';
  el.banners.innerHTML = '';

  try {
    const res = await fetch('/api/scan', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path }),
    });
    const data = await res.json();
    if (!res.ok) {
      banner('degraded', 'scan', data.error || res.statusText);
      return;
    }
    state.result = data;
    state.selected = null;
    render(data);
  } catch (err) {
    banner('degraded', 'scan', `Request failed: ${err.message}`);
  } finally {
    el.scan.disabled = false;
    el.scan.textContent = 'Scan';
  }
}

function banner(kind, stage, detail) {
  const div = document.createElement('div');
  div.className = `banner ${kind === 'degraded' ? 'degraded' : ''}`;
  div.innerHTML = `<b>${esc(stage)}</b><span>${esc(detail)}</span>`;
  el.banners.appendChild(div);
}

// --------------------------------------------------------------------- render

function render(r) {
  for (const s of r.stages) {
    if (s.status !== 'ok') {
      banner(s.status === 'degraded' ? 'degraded' : 'warn', s.name, s.detail || s.status);
    }
  }
  renderStats(r);
  renderGraph(r);
  renderFindings(r);
  renderPackages(r);
  el.timing.textContent = `${r.files} files · ${r.lines.toLocaleString()} lines · ${r.durationMs} ms · ${r.scannedAt}`;
}

function renderStats(r) {
  const s = r.stats;
  const cards = [
    { k: 'Packages', v: s.total, s: `${s.declared} declared` },
    { k: 'Phantom', v: s.phantom, s: 'declared, never imported', cls: 'phantom' },
    { k: 'Undeclared', v: s.undeclared, s: 'imported, never declared', cls: 'undeclared' },
    { k: 'Cycles', v: s.cycles, s: `${r.analysis.maxDepth} max depth` },
    { k: 'Advisories', v: s.advisories, s: 'from intel stage' },
    { k: 'Ghost score', v: r.analysis.ghostScore.toFixed(1), s: 'heuristic, see design §5', cls: 'score' },
  ];
  el.stats.innerHTML = cards
    .map(
      (c) => `<div class="card ${c.cls || ''}">
        <div class="k">${esc(c.k)}</div>
        <div class="v">${esc(c.v)}</div>
        <div class="s">${esc(c.s)}</div>
      </div>`
    )
    .join('');
}

// --- graph layout -----------------------------------------------------------
// Presentation-only: layered by BFS distance from the root node. The analysis
// numbers themselves come from the Rust stage.

function layout(r) {
  const n = r.packages.length;
  const adj = Array.from({ length: n }, () => []);
  for (const e of r.edges) {
    if (e.from < n && e.to < n) adj[e.from].push(e.to);
  }
  const dist = new Array(n).fill(-1);
  dist[0] = 0;
  const queue = [0];
  while (queue.length) {
    const v = queue.shift();
    for (const w of adj[v]) {
      if (dist[w] < 0) {
        dist[w] = dist[v] + 1;
        queue.push(w);
      }
    }
  }
  for (let i = 0; i < n; i++) {
    if (dist[i] < 0) dist[i] = 1; // imported directly by source, not by the root
  }

  const layers = new Map();
  for (let i = 0; i < n; i++) {
    if (!layers.has(dist[i])) layers.set(dist[i], []);
    layers.get(dist[i]).push(i);
  }
  return { dist, layers };
}

function renderGraph(r) {
  const n = r.packages.length;
  if (!n) {
    el.graph.innerHTML = '<p class="empty">Nothing to draw.</p>';
    return;
  }

  const { dist, layers } = layout(r);
  const maxLayer = Math.max(...layers.keys());
  const W = 760;
  const rowGap = 34;
  const maxRows = Math.max(...[...layers.values()].map((v) => v.length));
  const H = Math.max(300, maxRows * rowGap + 56);
  const xStep = maxLayer === 0 ? 0 : (W - 150) / maxLayer;

  const pos = new Array(n);
  for (const [layer, nodes] of layers) {
    nodes.sort((a, b) => r.packages[a].name.localeCompare(r.packages[b].name));
    const totalH = (nodes.length - 1) * rowGap;
    nodes.forEach((idx, i) => {
      pos[idx] = {
        x: 78 + layer * xStep,
        y: H / 2 - totalH / 2 + i * rowGap,
      };
    });
  }

  const edges = r.edges
    .filter((e) => pos[e.from] && pos[e.to])
    .map((e) => {
      const a = pos[e.from];
      const b = pos[e.to];
      const mid = (a.x + b.x) / 2;
      const cls = e.kind === 'dev' ? 'edge dev' : e.kind === 'dep' ? 'edge dep' : 'edge';
      return `<path class="${cls}" d="M${a.x + 10},${a.y} C${mid},${a.y} ${mid},${b.y} ${b.x - 11},${b.y}" />`;
    })
    .join('');

  const nodes = r.packages
    .map((p, i) => {
      const { x, y } = pos[i];
      const isRoot = p.state === 'root';
      let shape;
      if (isRoot) {
        shape = `<circle cx="${x}" cy="${y}" r="9" fill="var(--accent-2)" />`;
      } else if (p.state === 'phantom') {
        shape = `<path d="M${x - 7},${y + 9} v-9 a7,7 0 0 1 14,0 v9 z" fill="var(--phantom)" opacity="0.92" />`;
      } else if (p.state === 'undeclared') {
        shape = `<path d="M${x},${y - 9} l9,9 l-9,9 l-9,-9 z" fill="var(--undeclared)" opacity="0.92" />`;
      } else {
        shape = `<circle cx="${x}" cy="${y}" r="6.5" fill="var(--live)" opacity="0.9" />`;
      }
      const label = p.name.length > 22 ? `${p.name.slice(0, 21)}…` : p.name;
      const anchor = dist[i] === maxLayer ? 'end' : 'start';
      const dx = dist[i] === maxLayer ? -14 : 14;
      return `<g class="node-hit" data-idx="${i}">
          ${shape}
          <text class="node-label" x="${x + dx}" y="${y + 3.5}" text-anchor="${anchor}">${esc(label)}</text>
        </g>`;
    })
    .join('');

  el.graph.innerHTML = `<svg viewBox="0 0 ${W} ${H}" role="img" aria-label="Dependency graph with ${n} nodes">
      ${edges}
      ${nodes}
    </svg>`;

  el.graph.querySelectorAll('.node-hit').forEach((g) => {
    g.addEventListener('click', () => {
      state.selected = Number(g.dataset.idx);
      renderDetail();
    });
  });
}

function renderDetail() {
  const r = state.result;
  if (!r || state.selected == null) {
    el.detail.innerHTML = '';
    return;
  }
  const p = r.packages[state.selected];
  const advisories = (p.advisories || [])
    .map((a) => `<div class="adv">${esc(a.id)} · ${esc(a.severity)} — ${esc(a.summary)}</div>`)
    .join('');
  el.detail.innerHTML = `
    <h3>${esc(p.name)}</h3>
    <div class="row">ecosystem <b>${esc(p.ecosystem)}</b> · state <b>${esc(p.state)}</b>
      · declared <b>${p.declared ? 'yes' : 'no'}</b> · imported in <b>${p.files || 0}</b> file(s)</div>
    <div class="row">version <b>${esc(p.version || '—')}</b> · licence <b>${esc(p.license || 'unknown')}</b></div>
    ${advisories}`;
}

// --- findings / table -------------------------------------------------------

function renderFindings(r) {
  el.findingCount.textContent = `${r.findings.length}`;
  if (!r.findings.length) {
    el.findings.innerHTML = '<li class="empty">No defects found. Suspicious.</li>';
    return;
  }
  el.findings.innerHTML = r.findings
    .map(
      (f) => `<li>
        <div class="head">
          <span class="sev ${esc(f.severity)}">${esc(f.severity)}</span>
          <span class="title">${esc(f.title)}</span>
        </div>
        <div class="body">${esc(f.detail)}${f.file ? ` <span class="muted">(${esc(f.file)})</span>` : ''}</div>
      </li>`
    )
    .join('');
}

function renderPackages(r) {
  el.packageCount.textContent = `${r.stats.total} nodes`;
  el.packages.innerHTML = r.packages
    .map(
      (p) => `<tr>
        <td>${esc(p.name)}</td>
        <td class="muted">${esc(p.ecosystem)}</td>
        <td>${p.declared ? '✓' : '<span class="muted">—</span>'}</td>
        <td>${p.imported ? '✓' : '<span class="muted">—</span>'}</td>
        <td class="muted">${p.files || 0}</td>
        <td class="muted">${esc(p.version || '—')}</td>
        <td class="muted">${esc(p.license || '—')}</td>
        <td><span class="tag ${esc(p.state)}">${esc(p.state)}</span></td>
      </tr>`
    )
    .join('');
}

// ----------------------------------------------------------------------- init

el.form.addEventListener('submit', (e) => {
  e.preventDefault();
  runScan();
});

loadHealth();
loadFixtures();
