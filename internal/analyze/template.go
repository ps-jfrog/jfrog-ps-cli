package analyze

const css = `
:root {
  --bg:#f4f7fb; --surface:#fff; --ink:#172033; --muted:#667085;
  --line:#d8e0ec; --blue:#1769e0; --blue-bg:#eaf2ff;
  --green:#16794b; --green-bg:#e9f8f0; --amber:#9a6700;
  --amber-bg:#fff5d9; --red:#b42318; --red-bg:#ffefed;
  --purple:#6941c6; --purple-bg:#f1ecff; --teal:#087e8b;
  --teal-bg:#e7f8fa; --slate:#344054; --slate-bg:#eef1f5;
}
* { box-sizing:border-box; }
body { margin:0; background:var(--bg); color:var(--ink);
  font:14px/1.5 Inter, ui-sans-serif, system-ui, -apple-system, sans-serif; }
header { background:#111827; color:white; padding:30px 5vw 26px; }
.header-top { display:flex; justify-content:space-between; align-items:flex-start; gap:16px; flex-wrap:wrap; }
header h1 { margin:0 0 7px; font-size:25px; }
header p { margin:0; color:#c5cfdd; max-width:950px; }
.header-instance { color:#c5cfdd; font-size:13px; text-decoration:none; border:1px solid rgba(255,255,255,.25);
  border-radius:8px; padding:6px 12px; white-space:nowrap; flex-shrink:0; }
.header-instance:hover { border-color:rgba(255,255,255,.5); color:white; }
main { max-width:1500px; margin:auto; padding:24px 5vw 50px; }
h2 { margin:28px 0 12px; font-size:20px; }
.stats,.findings { display:grid; gap:12px; }
.stats { grid-template-columns:repeat(auto-fit,minmax(155px,1fr)); margin-top:-8px; }
.stat { background:var(--surface); border:1px solid var(--line);
  border-top:4px solid var(--blue); border-radius:10px; padding:16px; }
.stat strong { display:block; font-size:25px; line-height:1.1; }
.stat span { color:var(--muted); font-size:12px; }
.findings { grid-template-columns:repeat(auto-fit,minmax(205px,1fr)); }
.finding { border:1px solid transparent; border-radius:10px; padding:15px;
  text-align:left; cursor:pointer; color:var(--ink); transition:transform .12s; }
.finding:hover { transform:translateY(-2px); }
.finding strong { display:block; font-size:22px; }
.finding.active { outline:2px solid var(--blue); }
.warning,.amber { background:var(--amber-bg); border-color:#e7c75b; }
.danger,.red { background:var(--red-bg); border-color:#efaca7; }
.info { background:var(--blue-bg); border-color:#a8c8f5; }
.success,.green { background:var(--green-bg); border-color:#a9dbc2; }
.purple { background:var(--purple-bg); border-color:#c9b8f5; }
.teal { background:var(--teal-bg); border-color:#9bd7dd; }
.explorer { margin-top:28px; background:var(--surface); border:1px solid var(--line);
  border-radius:12px; overflow:hidden; }
.toolbar { display:flex; flex-wrap:wrap; gap:8px; padding:14px;
  background:#f9fafb; border-bottom:1px solid var(--line); align-items:center; }
button,select,input { font:inherit; }
.tab,.clear { border:1px solid var(--line); border-radius:7px;
  background:white; padding:7px 12px; cursor:pointer; }
.tab.active { background:var(--blue); color:white; border-color:var(--blue); }
input[type=search] { min-width:260px; flex:1; border:1px solid var(--line);
  border-radius:7px; padding:8px 10px; }
select { border:1px solid var(--line); border-radius:7px; padding:8px; background:white; }
.active-filter { padding:10px 14px; background:var(--blue-bg); color:#174a91;
  border-bottom:1px solid #bdd3f2; display:none; }
.active-filter.show { display:block; }
.table-wrap { overflow:auto; max-height:620px; }
table { width:100%; border-collapse:collapse; white-space:nowrap; }
th,td { padding:9px 11px; border-bottom:1px solid #edf0f4; text-align:left; }
th { position:sticky; top:0; z-index:1; background:#f7f9fc; font-size:12px; }
tbody tr { cursor:pointer; }
tbody tr:hover { background:#f5f9ff; }
.number { text-align:right; font-variant-numeric:tabular-nums; }
.badge { display:inline-block; border-radius:999px; padding:2px 7px;
  font-size:11px; margin:1px 3px 1px 0; background:var(--slate-bg); }
.badge.no-cleanup-policy,.badge.stale { background:var(--amber-bg); color:var(--amber); }
.badge.huge,.badge.massive-files { background:var(--red-bg); color:var(--red); }
.badge.naming-nonconformant,.badge.underscore,.badge.uppercase,.badge.multi-policy-match,.badge.very-long-name {
  background:var(--purple-bg); color:var(--purple); }
.badge.empty { background:var(--teal-bg); color:var(--teal); }
.drawer { display:none; padding:16px; border-top:1px solid var(--line);
  background:#fbfcfe; }
.drawer.show { display:block; }
.drawer-grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(180px,1fr)); gap:10px; }
.drawer dt { font-size:11px; color:var(--muted); text-transform:uppercase; }
.drawer dd { margin:2px 0 0; overflow-wrap:anywhere; }
.foot { color:var(--muted); margin-top:18px; font-size:12px; }
.report-tabs { display:flex; gap:8px; flex-wrap:wrap; position:sticky; top:0; z-index:5;
  padding:10px 0; margin-bottom:12px; background:var(--bg); border-bottom:1px solid var(--line); }
.report-tab { border:1px solid var(--line); border-radius:8px; background:white;
  color:var(--slate); padding:9px 16px; cursor:pointer; font-weight:650; }
.report-tab.active { color:white; background:var(--blue); border-color:var(--blue); }
.report-panel { display:none; }
.report-panel.active { display:block; }
.instance-line { font-size:15px; margin:-4px 0 20px; }
.instance-line a { color:var(--blue); }
.callout { border-radius:10px; padding:14px 16px; border:1px solid var(--line); margin-bottom:10px; }
.callout ul { margin:6px 0 0; padding-left:20px; }
.callout.ok { background:var(--green-bg); border-color:#a9dbc2; }
.callout.warn { background:var(--amber-bg); border-color:#e7c75b; }
.callout.info { background:var(--blue-bg); border-color:#a8c8f5; }
.gt-toolbar { display:flex; gap:10px; align-items:center; padding:12px 14px;
  background:#f9fafb; border:1px solid var(--line); border-bottom:none;
  border-radius:12px 12px 0 0; }
.gt-toolbar input[type=search] { min-width:260px; }
.gt-count { color:var(--muted); font-size:12px; white-space:nowrap; }
.generic-table { border:1px solid var(--line); border-radius:0 0 12px 12px; overflow:hidden; margin-bottom:24px; }
.generic-table .table-wrap { max-height:520px; }
.generic-table td { max-width:360px; overflow:hidden; text-overflow:ellipsis; }
.rel-toolbar { display:flex; gap:10px; align-items:center; padding:14px 0; flex-wrap:wrap; }
.rel-toolbar select { min-width:320px; }
.rel-toolbar input[type=search] { min-width:0; flex:none; width:220px; }
.rel-summary { color:var(--muted); font-size:13px; margin-bottom:10px; }
.rel-diagram-wrap { background:var(--surface); border:1px solid var(--line); border-radius:12px;
  padding:8px; margin-bottom:20px; }
.rel-svg { display:block; width:100%; height:auto; max-height:440px; }
.rel-edge { stroke:var(--line); stroke-width:1.5; }
.rel-node { fill:var(--blue-bg); stroke:var(--blue); stroke-width:1.5; }
.rel-node-center { fill:var(--blue); stroke:var(--blue); }
.rel-label { fill:var(--ink); font-size:11px; }
.rel-label-center { fill:white; font-weight:650; font-size:12px; }
.rel-more { color:var(--muted); font-size:12px; text-align:center; margin:6px 0 2px; }
.wave-toolbar { display:flex; flex-wrap:wrap; gap:8px; align-items:center; margin-bottom:14px; }
.wave-toolbar input[type=search] { min-width:220px; }
.wave-summary { color:var(--muted); font-size:12px; margin-left:auto; white-space:nowrap; }
.wave-criteria { background:var(--surface); border:1px solid var(--line); border-radius:12px;
  padding:12px 14px; margin-bottom:16px; display:flex; flex-direction:column; gap:10px; }
.wave-criteria-row { display:flex; flex-wrap:wrap; gap:10px; align-items:center; }
.wave-criteria-label { font-size:12px; font-weight:650; color:var(--slate); min-width:74px; }
.wave-finding-chips { display:flex; flex-wrap:wrap; gap:6px; flex:1; }
.wave-chip { border:1px solid var(--line); border-radius:999px; background:white;
  padding:5px 12px; font-size:12px; cursor:pointer; color:var(--ink); }
.wave-chip.active { background:var(--blue); color:white; border-color:var(--blue); }
.wave-mode-toggle { display:flex; gap:0; border:1px solid var(--line); border-radius:7px; overflow:hidden; flex-shrink:0; }
.wave-mode-btn { border:none; background:white; padding:7px 12px; font-size:12px; cursor:pointer; color:var(--slate); }
.wave-mode-btn.active { background:var(--slate); color:white; }
.wave-activity-custom input { width:64px; border:1px solid var(--line); border-radius:7px; padding:6px 8px; }
.wave-match-count { color:var(--muted); font-size:12px; white-space:nowrap; }
.wave-range-field { display:flex; align-items:center; gap:6px; font-size:12px; color:var(--slate); }
.wave-range-field input { width:70px; border:1px solid var(--line); border-radius:7px; padding:6px 8px; }
.wave-board { display:flex; gap:14px; overflow-x:auto; padding-bottom:8px; align-items:flex-start; }
.wave-column { background:var(--surface); border:1px solid var(--line); border-radius:12px;
  min-width:260px; max-width:300px; flex:0 0 auto; display:flex; flex-direction:column; max-height:720px; }
.wave-column-header { display:flex; align-items:center; gap:8px; padding:10px 12px;
  border-bottom:1px solid var(--line); background:#f9fafb; border-radius:12px 12px 0 0; }
.wave-column-name { font-weight:650; flex:1; overflow:hidden; text-overflow:ellipsis; }
.wave-column-name-input { flex:1; border:1px solid transparent; border-radius:6px; padding:3px 5px;
  font-weight:650; background:transparent; }
.wave-column-name-input:hover,.wave-column-name-input:focus { border-color:var(--line); background:white; }
.wave-column-count { color:var(--muted); font-size:12px; }
.wave-column-remove { border:none; background:transparent; color:var(--muted); cursor:pointer;
  font-size:16px; line-height:1; padding:0 2px; }
.wave-column-remove:hover { color:var(--red); }
.wave-column-body { padding:10px; overflow-y:auto; flex:1; display:flex; flex-direction:column; gap:8px; min-height:80px; }
.wave-column.drag-over .wave-column-body { background:var(--blue-bg); }
.wave-card { border:1px solid var(--line); border-radius:9px; padding:9px 10px; background:#fbfcfe; cursor:grab; }
.wave-card.dragging { opacity:.4; }
.wave-card-title { font-weight:650; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.wave-card-meta { color:var(--muted); font-size:11px; margin-top:2px; }
.wave-card-badges { margin-top:4px; }
.wave-card-move { margin-top:6px; width:100%; font-size:11px; padding:3px 4px; }
.wave-empty { color:var(--muted); font-size:12px; text-align:center; padding:16px 4px; border:1px dashed var(--line); border-radius:8px; }
.mig-config { display:flex; flex-wrap:wrap; gap:16px; align-items:flex-end; margin-bottom:16px;
  background:var(--surface); border:1px solid var(--line); border-radius:12px; padding:14px 16px; }
.mig-config-field { display:flex; flex-direction:column; gap:4px; font-size:12px; font-weight:650; color:var(--slate); }
.mig-config-field select { font-size:13px; padding:7px 9px; border:1px solid var(--line); border-radius:7px; min-width:260px; }
.mig-config-checkbox { flex-direction:row; align-items:center; gap:8px; font-weight:650; }
.mig-config-checkbox input { width:16px; height:16px; }
.mig-wave-list { margin:6px 0 0; padding-left:20px; }
.mig-phase-grid { display:grid; gap:12px; grid-template-columns:repeat(auto-fit,minmax(220px,1fr)); margin-bottom:20px; }
.mig-phase { border:1px solid var(--line); border-radius:10px; padding:14px; border-top:4px solid var(--blue); background:var(--surface); }
.mig-phase span { font-size:11px; text-transform:uppercase; color:var(--muted); font-weight:650; }
.mig-phase h4 { margin:4px 0 6px; font-size:15px; }
.mig-phase p { margin:0; font-size:12.5px; color:var(--slate); }
.mig-raci-table { width:100%; border-collapse:collapse; margin-bottom:12px; }
.mig-raci-table th,.mig-raci-table td { border-bottom:1px solid var(--line); padding:8px 10px; text-align:left; font-size:13px; }
.mig-raci-table th { background:#f7f9fc; font-size:11px; text-transform:uppercase; color:var(--muted); }
.mig-commands { background:#0b1220; color:#d7e3f4; border-radius:10px; padding:14px 16px;
  font-size:12.5px; overflow-x:auto; white-space:pre; }
.seq-svg { display:block; width:100%; height:auto; }
.seq-lane-box { fill:#eef1f5; stroke:#98a2b3; }
.seq-lane-label { font-size:12px; font-weight:700; fill:#172033; }
.seq-lifeline { stroke:#98a2b3; stroke-dasharray:4 5; }
.seq-msg { stroke:#344054; stroke-width:1.6; }
.seq-msg-blue { stroke:var(--blue); }
.seq-msg-purple { stroke:var(--purple); }
.seq-msg-green { stroke:var(--green); }
.seq-msg-danger { stroke:var(--red); }
.seq-msg-label { font-size:10.5px; fill:#344054; text-anchor:middle; }
@media (max-width:700px) { header,main { padding-left:18px; padding-right:18px; } }
`

// relationshipsScript is the Relationships tab's own script body (assumes
// a const REL_DATA = {projects:[...], repos:[...]} literal precedes it -
// see renderRelationshipsPanel in instance_panels.go). Kept separate from
// css/pageTemplate above since it's only emitted when an instance
// snapshot is present, unlike the always-present shared helpers.
const relationshipsScript = `
function relConnections(value) {
  if (value === "global") {
    return {
      title: "Global (repos with no project)",
      items: REL_DATA.repos.filter(r => !r.project_key || r.project_key === "default"),
      columns: [{key: "name", label: "Repo"}, {key: "package_type", label: "Package"}, {key: "repo_type", label: "Type"}],
      labelFn: r => r.name,
    };
  }
  if (value.indexOf("project:") === 0) {
    const key = value.slice(8);
    const proj = REL_DATA.projects.find(p => p.key === key);
    return {
      title: "Project: " + (proj && proj.display_name ? proj.display_name + " (" + key + ")" : key),
      items: REL_DATA.repos.filter(r => r.project_key === key),
      columns: [{key: "name", label: "Repo"}, {key: "package_type", label: "Package"}, {key: "repo_type", label: "Type"}],
      labelFn: r => r.name,
    };
  }
  return {title: "", items: [], columns: [], labelFn: () => ""};
}

function updateRelationships() {
  const select = document.getElementById("rel-select");
  if (!select) return;
  const info = relConnections(select.value);
  document.getElementById("rel-summary").textContent =
    info.title + " → " + info.items.length.toLocaleString() + " connection(s)";
  renderRelDiagram("rel-diagram", info.title, info.items, info.labelFn);
  mountTable("rel-table", info.items, info.columns);
}

// filterProjectOptions narrows the <select>'s "Projects" optgroup to options
// whose text matches the search box - Global stays visible, always. Returns
// the first still-visible project option (or null), so Enter can jump to it
// when the search has narrowed things to an obvious single match.
function filterProjectOptions() {
  const input = document.getElementById("rel-project-search");
  const select = document.getElementById("rel-select");
  if (!input || !select) return null;
  const term = input.value.trim().toLowerCase();
  const group = select.querySelector('optgroup[label="Projects"]');
  if (!group) return null;
  let firstVisible = null;
  Array.prototype.forEach.call(group.querySelectorAll("option"), option => {
    const match = !term || option.textContent.toLowerCase().indexOf(term) !== -1;
    option.hidden = !match;
    if (match && !firstVisible) firstVisible = option;
  });
  return firstVisible;
}

document.getElementById("rel-select").addEventListener("change", updateRelationships);
document.getElementById("rel-project-search").addEventListener("input", filterProjectOptions);
document.getElementById("rel-project-search").addEventListener("keydown", event => {
  if (event.key !== "Enter") return;
  event.preventDefault();
  const match = filterProjectOptions();
  if (!match) return;
  document.getElementById("rel-select").value = match.value;
  updateRelationships();
});
updateRelationships();
`

// wavePlanningPanel is the Wave Planning tab's inner content: a toolbar
// plus a kanban-style board (Backlog / one or more named waves / Excluded)
// that the viewer drags repo cards between to plan cleanup or migration
// batches, then exports as CSV. It needs no Go-side data of its own - it
// reads the same global DATA/FINDING_DEFS the Repos tab uses - so unlike
// the instance-derived panels this is always present, with or without an
// instance snapshot.
//
// Its own <script> block only *defines* initWavePlanning(); the call that
// actually runs it lives in pageTemplate's bottom script, after DATA/
// FINDING_DEFS/renderBadges exist - definition order across <script> tags
// doesn't matter in a browser, only call order does.
const wavePlanningPanel = `
  <h2>Wave planning</h2>
  <p style="color:var(--muted);margin:-4px 0 16px;max-width:820px;">
    Drag repositories out of the backlog into a wave to plan cleanup/migration batches - combine
    findings and last-push activity below to surface candidates, then drag them individually or
    bulk-assign everything currently matching. Assignments only live in this browser tab for this
    session - export the curated list before closing it.
  </p>
  <div class="wave-toolbar">
    <input id="wave-search" type="search" placeholder="Search backlog...">
    <select id="wave-sort">
      <option value="last_push_date">Sort: Last push (oldest first)</option>
      <option value="last_activity_date">Sort: Last activity (oldest first)</option>
      <option value="usage_gb">Sort: Usage GB (largest first)</option>
      <option value="repo_name">Sort: Name</option>
    </select>
    <button class="clear" id="wave-add">+ Add wave</button>
    <button class="clear" id="wave-export">Export CSV</button>
    <span class="wave-summary" id="wave-summary"></span>
  </div>
  <div class="wave-criteria">
    <div class="wave-criteria-row">
      <span class="wave-criteria-label">Repo type</span>
      <div class="wave-finding-chips" id="wave-type-chips"></div>
      <span style="color:var(--muted);font-size:12px;">Migrations typically move LOCAL repos only - remotes/virtuals are usually recreated, not copied.</span>
    </div>
    <div class="wave-criteria-row">
      <span class="wave-criteria-label">Findings</span>
      <div class="wave-finding-chips" id="wave-finding-chips"></div>
      <div class="wave-mode-toggle" id="wave-mode-toggle">
        <button type="button" class="wave-mode-btn active" data-mode="or">Match Any (OR)</button>
        <button type="button" class="wave-mode-btn" data-mode="and">Match All (AND)</button>
      </div>
    </div>
    <div class="wave-criteria-row">
      <span class="wave-criteria-label">Last push</span>
      <select id="wave-activity">
        <option value="">Any time</option>
        <option value="week">Within 1 week</option>
        <option value="month">Within 1 month</option>
        <option value="3months">Within 3 months</option>
        <option value="6months">Within 6 months</option>
        <option value="12months">Within 12 months</option>
        <option value="over1y">Over 1 year ago</option>
        <option value="custom">Over N years ago...</option>
      </select>
      <span id="wave-activity-custom" class="wave-activity-custom" style="display:none;">
        <input id="wave-activity-years" type="number" min="1" step="1" value="2"> year(s)
      </span>
    </div>
    <div class="wave-criteria-row">
      <span class="wave-criteria-label">Size &amp; files</span>
      <select id="wave-empty-filter">
        <option value="">Empty or not</option>
        <option value="nonempty">Non-empty only</option>
        <option value="empty">Empty only</option>
      </select>
      <label class="wave-range-field">Usage GB
        <input id="wave-usage-min" type="number" min="0" step="any" placeholder="min"> to
        <input id="wave-usage-max" type="number" min="0" step="any" placeholder="max">
      </label>
      <label class="wave-range-field">Files
        <input id="wave-files-min" type="number" min="0" step="1" placeholder="min"> to
        <input id="wave-files-max" type="number" min="0" step="1" placeholder="max">
      </label>
    </div>
    <div class="wave-criteria-row">
      <span class="wave-criteria-label">Bulk assign</span>
      <span class="wave-match-count" id="wave-match-count"></span>
      <select id="wave-assign-target"></select>
      <button class="clear" id="wave-assign-btn">Assign matching backlog repos</button>
    </div>
  </div>
  <div class="wave-board" id="wave-board"></div>
  <script>
  function initWavePlanning() {
    const board = document.getElementById("wave-board");
    if (!board) return;
    const search = document.getElementById("wave-search");
    const sortSelect = document.getElementById("wave-sort");
    const addBtn = document.getElementById("wave-add");
    const exportBtn = document.getElementById("wave-export");
    const summary = document.getElementById("wave-summary");
    const typeChipsBox = document.getElementById("wave-type-chips");
    const chipsBox = document.getElementById("wave-finding-chips");
    const modeToggle = document.getElementById("wave-mode-toggle");
    const activitySelect = document.getElementById("wave-activity");
    const activityCustom = document.getElementById("wave-activity-custom");
    const activityYears = document.getElementById("wave-activity-years");
    const emptyFilter = document.getElementById("wave-empty-filter");
    const usageMin = document.getElementById("wave-usage-min");
    const usageMax = document.getElementById("wave-usage-max");
    const filesMin = document.getElementById("wave-files-min");
    const filesMax = document.getElementById("wave-files-max");
    const matchCountEl = document.getElementById("wave-match-count");
    const assignTarget = document.getElementById("wave-assign-target");
    const assignBtn = document.getElementById("wave-assign-btn");

    let waveList = [{id: "wave-1", name: "Wave 1"}];
    let waveNextId = 2;
    const waveAssignment = {}; // repo_name -> column id ("wave-N" or "excluded"); absent = backlog
    const selectedFindings = new Set();
    let findingMode = "or"; // "or" = match any selected finding, "and" = match all of them

    // Repo-type chips: default to LOCAL only when the estate has one (the
    // usual migration scope), otherwise default to every type present so
    // the backlog isn't empty on load.
    const allTypes = [...new Set(DATA.map(r => r.repo_type).filter(Boolean))].sort();
    const selectedTypes = new Set(allTypes.includes("LOCAL") ? ["LOCAL"] : allTypes);
    typeChipsBox.innerHTML = allTypes.map(t =>
      '<button type="button" class="wave-chip' + (selectedTypes.has(t) ? " active" : "") + '" data-type="' + escapeHtml(t) + '">' + escapeHtml(t) + '</button>'
    ).join('');
    typeChipsBox.querySelectorAll(".wave-chip").forEach(chip => {
      chip.addEventListener("click", () => {
        const t = chip.dataset.type;
        if (selectedTypes.has(t)) selectedTypes.delete(t); else selectedTypes.add(t);
        chip.classList.toggle("active", selectedTypes.has(t));
        render();
      });
    });
    // No type selected at all means "show nothing," not "show everything" -
    // an explicit empty facet shouldn't silently fall back to unfiltered.
    function matchesType(r) { return selectedTypes.has(r.repo_type); }

    chipsBox.innerHTML = FINDING_DEFS.map(d =>
      '<button type="button" class="wave-chip" data-finding="' + escapeHtml(d.ID) + '">' + escapeHtml(d.Label) + '</button>'
    ).join('');
    chipsBox.querySelectorAll(".wave-chip").forEach(chip => {
      chip.addEventListener("click", () => {
        const id = chip.dataset.finding;
        if (selectedFindings.has(id)) selectedFindings.delete(id); else selectedFindings.add(id);
        chip.classList.toggle("active", selectedFindings.has(id));
        render();
      });
    });
    modeToggle.querySelectorAll(".wave-mode-btn").forEach(btn => {
      btn.addEventListener("click", () => {
        findingMode = btn.dataset.mode;
        modeToggle.querySelectorAll(".wave-mode-btn").forEach(b => b.classList.toggle("active", b === btn));
        render();
      });
    });
    activitySelect.addEventListener("change", () => {
      activityCustom.style.display = activitySelect.value === "custom" ? "" : "none";
      render();
    });
    activityYears.addEventListener("input", render);

    // matchesSizeFiles: an empty min/max input leaves that bound off. The
    // empty/non-empty select is a shortcut for the common "files_count==0"
    // case, independent of (and combinable with) the numeric ranges below.
    function matchesSizeFiles(r) {
      const files = Number(r.files_count) || 0;
      const usage = Number(r.usage_gb) || 0;
      if (emptyFilter.value === "empty" && files !== 0) return false;
      if (emptyFilter.value === "nonempty" && files === 0) return false;
      if (usageMin.value !== "" && usage < Number(usageMin.value)) return false;
      if (usageMax.value !== "" && usage > Number(usageMax.value)) return false;
      if (filesMin.value !== "" && files < Number(filesMin.value)) return false;
      if (filesMax.value !== "" && files > Number(filesMax.value)) return false;
      return true;
    }
    emptyFilter.addEventListener("change", render);
    [usageMin, usageMax, filesMin, filesMax].forEach(input => input.addEventListener("input", render));

    const columns = () => [{id: "backlog", name: "Backlog", removable: false}]
      .concat(waveList.map(w => ({id: w.id, name: w.name, removable: true})))
      .concat([{id: "excluded", name: "Excluded", removable: false}]);

    // matchesFindings: with nothing selected, every repo passes (the
    // findings criteria is simply inactive). "or" matches a repo tagged
    // with at least one selected finding; "and" requires all of them.
    function matchesFindings(r) {
      if (selectedFindings.size === 0) return true;
      const rf = r.findings || [];
      const ids = [...selectedFindings];
      return findingMode === "and" ? ids.every(f => rf.indexOf(f) !== -1) : ids.some(f => rf.indexOf(f) !== -1);
    }

    // daysSincePush returns null when last_push_date is missing/unparsable
    // (no recorded push) rather than 0, so "within the last N" filters
    // below correctly treat "never recorded" as NOT recent.
    function daysSincePush(r) {
      if (!r.last_push_date) return null;
      const t = Date.parse(r.last_push_date);
      if (isNaN(t)) return null;
      return (Date.now() - t) / 86400000;
    }

    // matchesActivity is evaluated against the viewer's current clock, so
    // "within 1 month" etc. always reflects when the report is opened, not
    // when it was generated. An unrecorded push date only satisfies the
    // "over ... ago" filters (over1y/custom), never the "within" ones,
    // since there is nothing recorded to call recent.
    function matchesActivity(r) {
      const mode = activitySelect.value;
      if (!mode) return true;
      const days = daysSincePush(r);
      if (mode === "over1y") return days === null || days > 365;
      if (mode === "custom") {
        const years = Math.max(1, Number(activityYears.value) || 1);
        return days === null || days > years * 365;
      }
      const withinDays = {week: 7, month: 30, "3months": 90, "6months": 180, "12months": 365}[mode];
      return days !== null && days <= withinDays;
    }

    function reposFor(colId) {
      let list;
      if (colId === "backlog") {
        list = DATA.filter(r => !waveAssignment[r.repo_name]);
        const q = search.value.trim().toLowerCase();
        if (q) list = list.filter(r => JSON.stringify(r).toLowerCase().indexOf(q) !== -1);
        list = list.filter(matchesType).filter(matchesFindings).filter(matchesActivity).filter(matchesSizeFiles);
      } else {
        list = DATA.filter(r => waveAssignment[r.repo_name] === colId);
      }
      const key = sortSelect.value;
      return list.slice().sort((a, b) => {
        if (key === "repo_name") return String(a.repo_name).localeCompare(String(b.repo_name));
        if (key === "usage_gb") return (Number(b.usage_gb) || 0) - (Number(a.usage_gb) || 0);
        const av = a[key], bv = b[key];
        if (!av) return -1;
        if (!bv) return 1;
        return String(av).localeCompare(String(bv));
      });
    }

    function moveOptionsHTML(repoName) {
      const current = waveAssignment[repoName] || "backlog";
      return columns().map(c =>
        '<option value="' + c.id + '"' + (c.id === current ? " selected" : "") + '>' + escapeHtml(c.name) + '</option>'
      ).join('');
    }

    function cardHTML(r) {
      const usage = Number(r.usage_gb || 0).toLocaleString(undefined, {maximumFractionDigits: 2});
      const type = escapeHtml(r.package_type || "") + (r.repo_type ? " · " + escapeHtml(r.repo_type) : "");
      return '<div class="wave-card" draggable="true" data-repo="' + escapeHtml(r.repo_name) + '">' +
        '<div class="wave-card-title">' + escapeHtml(r.repo_name) + '</div>' +
        '<div class="wave-card-meta">' + type + '</div>' +
        '<div class="wave-card-meta">' + usage + ' GB · last push: ' + escapeHtml(r.last_push_date || "unknown") + '</div>' +
        '<div class="wave-card-badges">' + renderBadges(r.findings) + '</div>' +
        '<select class="wave-card-move">' + moveOptionsHTML(r.repo_name) + '</select>' +
        '</div>';
    }

    function setAssignment(repoName, colId) {
      if (colId === "backlog") delete waveAssignment[repoName];
      else waveAssignment[repoName] = colId;
    }

    function render() {
      board.innerHTML = "";
      columns().forEach(col => {
        const repos = reposFor(col.id);
        const section = document.createElement("div");
        section.className = "wave-column";
        section.dataset.colId = col.id;

        const header = document.createElement("div");
        header.className = "wave-column-header";
        let nameEl;
        if (col.removable) {
          nameEl = document.createElement("input");
          nameEl.type = "text";
          nameEl.className = "wave-column-name-input";
          nameEl.value = col.name;
          nameEl.addEventListener("change", () => {
            const w = waveList.find(x => x.id === col.id);
            if (w) w.name = nameEl.value.trim() || w.name;
            render();
          });
        } else {
          nameEl = document.createElement("span");
          nameEl.className = "wave-column-name";
          nameEl.textContent = col.name;
        }
        header.appendChild(nameEl);
        const count = document.createElement("span");
        count.className = "wave-column-count";
        count.textContent = String(repos.length);
        header.appendChild(count);
        if (col.removable) {
          const removeBtn = document.createElement("button");
          removeBtn.className = "wave-column-remove";
          removeBtn.textContent = "×";
          removeBtn.title = "Remove wave (moves its repos back to Backlog)";
          removeBtn.addEventListener("click", () => {
            Object.keys(waveAssignment).forEach(name => {
              if (waveAssignment[name] === col.id) delete waveAssignment[name];
            });
            waveList = waveList.filter(w => w.id !== col.id);
            render();
          });
          header.appendChild(removeBtn);
        }
        section.appendChild(header);

        const body = document.createElement("div");
        body.className = "wave-column-body";
        body.addEventListener("dragover", e => { e.preventDefault(); section.classList.add("drag-over"); });
        body.addEventListener("dragleave", () => section.classList.remove("drag-over"));
        body.addEventListener("drop", e => {
          e.preventDefault();
          section.classList.remove("drag-over");
          const repoName = e.dataTransfer.getData("text/plain");
          if (repoName) { setAssignment(repoName, col.id); render(); }
        });
        body.innerHTML = repos.map(cardHTML).join("") ||
          '<div class="wave-empty">' + (col.id === "backlog" ? "No matching repos" : "Drag repos here") + '</div>';
        section.appendChild(body);
        board.appendChild(section);
      });

      board.querySelectorAll(".wave-card").forEach(card => {
        card.addEventListener("dragstart", e => {
          e.dataTransfer.setData("text/plain", card.dataset.repo);
          card.classList.add("dragging");
        });
        card.addEventListener("dragend", () => card.classList.remove("dragging"));
        const select = card.querySelector(".wave-card-move");
        select.setAttribute("draggable", "false");
        select.addEventListener("click", e => e.stopPropagation());
        select.addEventListener("change", () => { setAssignment(card.dataset.repo, select.value); render(); });
      });

      const targets = columns().filter(c => c.id !== "backlog");
      const prevTarget = assignTarget.value;
      assignTarget.innerHTML = targets.map(c => '<option value="' + c.id + '">' + escapeHtml(c.name) + '</option>').join('');
      if (targets.some(c => c.id === prevTarget)) assignTarget.value = prevTarget;

      const matching = reposFor("backlog");
      matchCountEl.textContent = matching.length + " matching backlog repo(s)";
      assignBtn.disabled = matching.length === 0;

      summary.textContent = Object.keys(waveAssignment).length + " of " + DATA.length + " repos assigned to a wave/excluded";
    }

    addBtn.addEventListener("click", () => {
      waveList.push({id: "wave-" + waveNextId, name: "Wave " + waveNextId});
      waveNextId++;
      render();
    });

    assignBtn.addEventListener("click", () => {
      const target = assignTarget.value;
      if (!target) return;
      reposFor("backlog").forEach(r => setAssignment(r.repo_name, target));
      render();
    });

    exportBtn.addEventListener("click", () => {
      const csvRows = [["repo_name", "wave", "package_type", "repo_type", "files_count", "usage_gb", "last_push_date", "last_activity_date", "findings"]];
      columns().forEach(col => {
        if (col.id === "backlog") return; // only export repos that were actually curated into a wave/excluded
        reposFor(col.id).forEach(r => csvRows.push([
          r.repo_name, col.name, r.package_type || "", r.repo_type || "",
          r.files_count != null ? r.files_count : "", r.usage_gb != null ? r.usage_gb : "",
          r.last_push_date || "", r.last_activity_date || "", (r.findings || []).join("|"),
        ]));
      });
      const csv = csvRows.map(row => row.map(cell => {
        const s = String(cell === undefined || cell === null ? "" : cell);
        return /[",\n]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s;
      }).join(",")).join("\n");
      const blob = new Blob([csv], {type: "text/csv;charset=utf-8;"});
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = "wave-plan.csv";
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    });

    search.addEventListener("input", render);
    sortSelect.addEventListener("change", render);

    // Exposed so the Migration Workflow tab's wave-based scope can list
    // current waves + counts live, without the two tabs sharing module
    // state any other way - recomputed fresh on every call.
    window.jfpsGetWavePlan = () => waveList.map(w => ({
      id: w.id, name: w.name,
      count: Object.values(waveAssignment).filter(v => v === w.id).length,
    }));

    render();
  }
  </script>
`

// migrationWorkflowPanel is the Migration Workflow tab's inner content: a
// small config form (config-transfer mode, all-repos-vs-wave-based scope,
// Terraform yes/no) that drives a phase-card summary, a "who does what"
// table, a generic swimlane sequence diagram, and a reference command
// block - all recomputed client-side on every control change. Needs no
// Go-side data of its own (same reasoning as wavePlanningPanel), so it's
// always present. Its wave-based scope reads live wave/assignment data
// out of Wave Planning via window.jfpsGetWavePlan() rather than the two
// tabs sharing any Go-rendered state.
const migrationWorkflowPanel = `
  <h2>Migration workflow</h2>
  <p style="color:var(--muted);margin:-4px 0 16px;max-width:820px;">
    A self-hosted -&gt; SaaS migration is config transfer, then binary transfer, then provisioning
    the governed target repos, then cutover. The three choices below change which commands run,
    who runs them, and whether it happens once for the whole estate or wave by wave.
  </p>
  <div class="mig-config">
    <label class="mig-config-field">
      <span>Repository config transfer</span>
      <select id="mig-transfer-mode">
        <option value="merge">Merge definitions (transfer-config-merge)</option>
        <option value="asis">As-is (transfer-config)</option>
      </select>
    </label>
    <label class="mig-config-field">
      <span>Migration scope</span>
      <select id="mig-scope">
        <option value="all">All eligible repos at once</option>
        <option value="wave">By wave (from Wave Planning)</option>
      </select>
    </label>
    <label class="mig-config-field mig-config-checkbox">
      <input type="checkbox" id="mig-terraform">
      <span>Customer provisions target repos/projects with Terraform</span>
    </label>
  </div>
  <div id="mig-wave-scope" class="callout info" style="display:none;"></div>

  <h3>Phases</h3>
  <div class="mig-phase-grid" id="mig-phases"></div>

  <h3>Who does what</h3>
  <div id="mig-raci"></div>

  <h3>Process sequence</h3>
  <div class="diagram-shell">
    <div id="mig-sequence"></div>
  </div>

  <h3>Reference commands</h3>
  <pre class="mig-commands" id="mig-commands"></pre>

  <script>
  function initMigrationWorkflow() {
    const transferModeSel = document.getElementById("mig-transfer-mode");
    const scopeSel = document.getElementById("mig-scope");
    const terraformCheckbox = document.getElementById("mig-terraform");
    const waveScopeBox = document.getElementById("mig-wave-scope");
    const phasesEl = document.getElementById("mig-phases");
    const raciEl = document.getElementById("mig-raci");
    const commandsEl = document.getElementById("mig-commands");
    if (!transferModeSel) return;

    const localCount = DATA.filter(r => r.repo_type === "LOCAL").length;
    const scopeNote = localCount > 0
      ? localCount + " of " + DATA.length + " repos in this report are LOCAL. Migration normally " +
        "targets local repos only - remotes are recreated against curated upstreams, virtuals are " +
        "recreated against the new locals/remotes, neither has artifacts to copy."
      : "";

    function buildPhases(cfg) {
      const phases = [];
      phases.push({
        kicker: "Phase 1", tone: "blue",
        title: cfg.transferMode === "asis" ? "Transfer config as-is" : "Merge config definitions",
        body: cfg.transferMode === "asis"
          ? "<code>jf rt transfer-config</code> copies repositories, users, groups, permissions, and security settings onto the target. Only safe against a brand-new, empty JPD - it can overwrite target Access/OIDC/admin configuration."
          : "<code>jf rt transfer-config-merge</code> copies just repository (and optionally project) definitions, scoped with <code>--include-repos</code>. Safe alongside a SaaS tenant that already has its own identity/security setup.",
      });
      phases.push({
        kicker: "Phase 2", tone: "amber",
        title: "Transfer binaries",
        body: "<code>jf rt transfer-files</code> moves artifact binaries and metadata for the selected scope (" +
          (cfg.scope === "wave" ? "one wave at a time, in wave order" : "all eligible repos in one pass") +
          "). Resumable; validate counts/checksums before moving on.",
      });
      phases.push({
        kicker: "Phase 3", tone: "purple",
        title: cfg.terraform ? "Provision governed repos (Terraform)" : "Create governed repos",
        body: cfg.terraform
          ? "<code>terraform apply</code> creates the target-state repos/projects/cleanup policies/roles declaratively - customer-owned, reviewable, repeatable."
          : "Target-state repos are created directly via the UI or REST API (by PS or the customer's platform team) - track this by hand since there is no declarative source of truth.",
      });
      phases.push({
        kicker: "Phase 4", tone: "green",
        title: "Copy, cutover, retire",
        body: "In-SaaS checksum copy (<code>jf rt copy</code> / <code>/api/copy</code>) from the landed repo into its governed target, client cutover, then retire the source key after a validated soak." +
          (cfg.scope === "wave" ? " Repeat this phase per wave, in wave order." : ""),
      });
      return phases;
    }

    function buildRaci(cfg) {
      return [
        {task: "Scope selection (repo types, waves)", owner: "Customer, with PS"},
        {task: cfg.transferMode === "asis" ? "Run transfer-config (as-is)" : "Run transfer-config-merge", owner: "PS / migration engineer"},
        {task: "Run transfer-files (binaries)", owner: "PS / migration engineer"},
        {task: cfg.terraform ? "Author & apply Terraform for target repos" : "Create target repos (UI/API)", owner: cfg.terraform ? "Customer (Terraform owner)" : "PS or customer platform team"},
        {task: "Validate counts / checksums", owner: "Joint - PS + customer"},
        {task: "Client cutover (resolvers/deployers/CI)", owner: "Customer"},
        {task: "Retire source keys", owner: "Customer, after PS sign-off"},
      ];
    }

    function laneNames(cfg) {
      const lanes = ["Customer / PS operator"];
      if (cfg.terraform) lanes.push("Terraform");
      lanes.push("Self-hosted JPD", "SaaS JPD");
      return lanes;
    }

    function buildMessages(cfg) {
      const L = {op: 0};
      let idx = 1;
      if (cfg.terraform) L.tf = idx++;
      L.sh = idx++;
      L.saas = idx++;

      const scopeLabel = cfg.scope === "wave" ? "this wave's repos" : "all eligible repos";
      const configCmd = cfg.transferMode === "asis" ? "transfer-config" : "transfer-config-merge --include-repos <" + scopeLabel + ">";

      const msgs = [];
      msgs.push({from: L.op, to: L.sh, label: "inventory + select repo types / " + scopeLabel});
      msgs.push({from: L.op, to: L.saas, label: "jf rt " + configCmd, tone: "blue"});
      if (cfg.transferMode === "asis") {
        msgs.push({from: L.saas, to: L.op, label: "warning: overwrites Access/users/OIDC on target", tone: "danger"});
      }
      msgs.push({from: L.op, to: L.sh, label: "jf rt transfer-files --include-repos <" + scopeLabel + ">"});
      msgs.push({from: L.sh, to: L.saas, label: "binaries + metadata", tone: "purple"});
      msgs.push({from: L.op, to: L.saas, label: "validate counts / checksums", tone: "green"});
      if (cfg.terraform) {
        msgs.push({from: L.op, to: L.tf, label: "terraform apply (governed repos/projects/cleanup)"});
        msgs.push({from: L.tf, to: L.saas, label: "create target repos", tone: "purple"});
      } else {
        msgs.push({from: L.op, to: L.saas, label: "create/adjust target repos via UI or REST API"});
      }
      msgs.push({from: L.op, to: L.saas, label: "jf rt copy / api/copy into governed repos", tone: "blue"});
      msgs.push({from: L.op, to: L.saas, label: "cutover clients; retire source keys", tone: "danger"});
      return msgs;
    }

    function buildCommands(cfg) {
      const scopeFlag = cfg.scope === "wave" ? '--include-repos "$WAVE_REPOS"' : '--include-repos "$ALL_ELIGIBLE_REPOS"';
      const lines = [];
      lines.push("# Connect both JPDs");
      lines.push('jf c add source --url "$SH_URL" --user "$SH_USER" --password "$SH_PASSWORD"');
      lines.push('jf c add target --url "$SAAS_URL" --access-token "$SAAS_TOKEN"');
      lines.push("");
      if (cfg.transferMode === "asis") {
        lines.push("# Config transfer (AS-IS) - only against a brand-new, empty target");
        lines.push("jf rt transfer-config source target");
      } else {
        lines.push("# Config transfer (MERGE) - safe alongside an existing SaaS identity/security setup");
        lines.push("jf rt transfer-config-merge source target " + scopeFlag);
      }
      lines.push("");
      lines.push("# Binary + metadata transfer" + (cfg.scope === "wave" ? " - repeat per wave, in wave order" : ""));
      lines.push("jf rt transfer-files source target " + scopeFlag);
      lines.push("jf rt transfer-files source target --status   # monitor from a second shell");
      lines.push("");
      if (cfg.terraform) {
        lines.push("# Provision target-state repos/projects (customer-owned Terraform)");
        lines.push("terraform plan -out=tfplan && terraform apply tfplan");
      } else {
        lines.push("# Create/adjust target-state repos directly (UI or REST API)");
        lines.push('jf rt curl --server-id target -XPOST "/api/repositories/<new-repo-key>" -d @repo.json');
      }
      lines.push("");
      lines.push("# In-SaaS copy into the governed repo, then cutover + retire");
      lines.push('jf rt copy "<landed-repo>/(*)" "<governed-repo>/{1}" --server-id target --flat=false');
      return lines.join("\n");
    }

    function render() {
      const cfg = {
        transferMode: transferModeSel.value,
        scope: scopeSel.value,
        terraform: terraformCheckbox.checked,
      };

      if (cfg.scope === "wave") {
        const waves = (window.jfpsGetWavePlan ? window.jfpsGetWavePlan() : []).filter(w => w.count > 0);
        waveScopeBox.style.display = "";
        waveScopeBox.innerHTML = waves.length === 0
          ? "<strong>No repos assigned to a wave yet.</strong> Visit the Wave Planning tab to build one, or switch scope back to 'All eligible repos at once'."
          : "<strong>" + waves.length + " wave(s) from Wave Planning:</strong> run phases 2-4 once per wave, in this order:" +
            '<ol class="mig-wave-list">' + waves.map(w => "<li>" + escapeHtml(w.name) + " - " + w.count + " repo(s)</li>").join("") + "</ol>";
      } else {
        waveScopeBox.style.display = "none";
      }

      phasesEl.innerHTML = buildPhases(cfg).map(p =>
        '<div class="mig-phase" style="border-top-color:var(--' + p.tone + ')"><span>' + escapeHtml(p.kicker) +
        '</span><h4>' + escapeHtml(p.title) + '</h4><p>' + p.body + '</p></div>'
      ).join('');

      raciEl.innerHTML = '<table class="mig-raci-table"><thead><tr><th>Task</th><th>Typically owned by</th></tr></thead><tbody>' +
        buildRaci(cfg).map(r => '<tr><td>' + r.task + '</td><td>' + r.owner + '</td></tr>').join('') +
        '</tbody></table>' + (scopeNote ? '<p style="color:var(--muted);font-size:12px;">' + escapeHtml(scopeNote) + '</p>' : '');

      renderSequenceDiagram("mig-sequence", laneNames(cfg), buildMessages(cfg));
      commandsEl.textContent = buildCommands(cfg);
    }

    transferModeSel.addEventListener("change", render);
    scopeSel.addEventListener("change", render);
    terraformCheckbox.addEventListener("change", render);

    // Re-render whenever this tab is opened too, so switching back after
    // building/changing waves in Wave Planning picks up the latest counts.
    const migTabBtn = document.querySelector('[data-report-tab="migration"]');
    if (migTabBtn) migTabBtn.addEventListener("click", render);

    render();
  }
  </script>
`

const pageTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>JFrog Artifactory Repository Audit</title>
  <style>__CSS__</style>
</head>
<body>
<script>
// Shared helpers, defined before <main> so the Overview/Access/Projects/
// Federation & Replication panels' inline mountTable(...) calls (which run
// as the browser parses <main>, before the bottom <script> block below)
// can find them.
function escapeHtml(value) {
  return String(value === undefined || value === null ? "" : value).replace(/[&<>"']/g, character => ({
    "&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#039;"
  })[character]);
}

function prettyLabel(key) {
  return key.replace(/_/g, " ").replace(/\b\w/g, c => c.toUpperCase());
}

function cellDisplay(value) {
  if (value === undefined || value === null || value === "") return "";
  if (Array.isArray(value)) {
    // Array.prototype.join calls each element's default toString(), which
    // yields "[object Object]" for an array of objects (e.g. federation
    // "members": [{url,enabled}, ...]) - stringify those individually.
    return value.map(v => (v !== null && typeof v === "object") ? JSON.stringify(v) : String(v)).join(", ");
  }
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function mountTable(containerId, data, columns) {
  const root = document.getElementById(containerId);
  if (!root) return;
  data = data || [];
  root.innerHTML =
    '<div class="gt-toolbar"><input type="search" placeholder="Search..." class="gt-search"><span class="gt-count"></span></div>' +
    '<div class="generic-table"><div class="table-wrap"><table><thead></thead><tbody></tbody></table></div>' +
    '<section class="drawer"></section></div>';
  const search = root.querySelector(".gt-search");
  const countEl = root.querySelector(".gt-count");
  const thead = root.querySelector("thead");
  const tbody = root.querySelector("tbody");
  const drawer = root.querySelector(".drawer");

  thead.innerHTML = "<tr>" + columns.map(c =>
    "<th" + (c.numeric ? ' class="number"' : "") + ">" + escapeHtml(c.label) + "</th>").join("") + "</tr>";

  function draw() {
    const q = search.value.trim().toLowerCase();
    const rows = data.filter(r => !q || JSON.stringify(r).toLowerCase().includes(q));
    countEl.textContent = rows.length.toLocaleString() + " / " + data.length.toLocaleString();
    tbody.innerHTML = rows.map((r, i) => '<tr data-index="' + i + '">' +
      columns.map(c => "<td" + (c.numeric ? ' class="number"' : "") + " title=\"" + escapeHtml(cellDisplay(r[c.key])) + "\">" +
        escapeHtml(cellDisplay(r[c.key])) + "</td>").join("") +
      "</tr>").join("");
    tbody.querySelectorAll("tr").forEach(tr => tr.addEventListener("click", () => {
      const row = rows[Number(tr.dataset.index)];
      const pairs = Object.entries(row);
      drawer.innerHTML = '<dl class="drawer-grid">' + pairs.map(([k, v]) =>
        "<div><dt>" + escapeHtml(prettyLabel(k)) + "</dt><dd>" +
        escapeHtml(cellDisplay(v) || "None") + "</dd></div>").join("") + "</dl>";
      drawer.classList.add("show");
      drawer.scrollIntoView({behavior: "smooth", block: "nearest"});
    }));
  }
  search.addEventListener("input", draw);
  draw();
}

function truncateLabel(value, max) {
  const s = String(value === undefined || value === null ? "" : value);
  max = max || 20;
  return s.length > max ? s.slice(0, max - 1) + "…" : s;
}

// renderRelDiagram draws a simple hub-and-spoke SVG: centerLabel in the
// middle, up to 24 of items around it (labelFn extracts each item's
// display text). Plain inline SVG, no charting library - consistent with
// this whole report being a single dependency-free HTML file.
function renderRelDiagram(containerId, centerLabel, items, labelFn) {
  const container = document.getElementById(containerId);
  if (!container) return;
  const cap = 24;
  const shown = items.slice(0, cap);
  const cx = 300, cy = 210, r = 165;
  let svg = '<svg viewBox="0 0 600 420" class="rel-svg" preserveAspectRatio="xMidYMid meet">';
  shown.forEach((item, i) => {
    const angle = (2 * Math.PI * i) / Math.max(shown.length, 1) - Math.PI / 2;
    const x = cx + r * Math.cos(angle);
    const y = cy + r * Math.sin(angle);
    svg += '<line x1="' + cx + '" y1="' + cy + '" x2="' + x + '" y2="' + y + '" class="rel-edge"></line>';
    svg += '<circle cx="' + x + '" cy="' + y + '" r="7" class="rel-node"></circle>';
    svg += '<text x="' + x + '" y="' + (y - 12) + '" class="rel-label" text-anchor="middle">' +
      escapeHtml(truncateLabel(labelFn(item), 16)) + '</text>';
  });
  svg += '<circle cx="' + cx + '" cy="' + cy + '" r="28" class="rel-node rel-node-center"></circle>';
  svg += '<text x="' + cx + '" y="' + (cy + 4) + '" class="rel-label rel-label-center" text-anchor="middle">' +
    escapeHtml(truncateLabel(centerLabel, 22)) + '</text>';
  svg += '</svg>';
  if (items.length > shown.length) {
    svg += '<p class="rel-more">Showing ' + shown.length + ' of ' + items.length + ' - see the table below for the full list.</p>';
  }
  container.innerHTML = svg;
}

// renderSequenceDiagram draws a generic swimlane sequence diagram: one
// lifeline per lane name, and an ordered list of {from, to, label, tone}
// messages (from/to are lane indexes) drawn as arrows down the page.
// Plain inline SVG, no charting library - same approach as
// renderRelDiagram, generalized to an arbitrary lane count so callers
// (e.g. the Migration Workflow tab) can reshape the diagram from config.
function renderSequenceDiagram(containerId, lanes, messages) {
  const container = document.getElementById(containerId);
  if (!container) return;
  const width = 1040;
  const laneGap = width / (lanes.length + 1);
  const laneX = lanes.map((_, i) => laneGap * (i + 1));
  const startY = 90, rowGap = 46;
  const height = startY + messages.length * rowGap + 30;
  const arrowId = "seq-arrow-" + containerId;
  let svg = '<svg viewBox="0 0 ' + width + ' ' + height + '" class="seq-svg" preserveAspectRatio="xMidYMid meet">';
  svg += '<defs><marker id="' + arrowId + '" markerWidth="10" markerHeight="8" refX="9" refY="4" orient="auto">' +
    '<path d="M0,0 L10,4 L0,8 Z" fill="#344054"></path></marker></defs>';
  lanes.forEach((name, i) => {
    const x = laneX[i];
    svg += '<rect x="' + (x - 78) + '" y="14" width="156" height="40" rx="7" class="seq-lane-box"></rect>';
    svg += '<text x="' + x + '" y="39" text-anchor="middle" class="seq-lane-label">' + escapeHtml(truncateLabel(name, 22)) + '</text>';
    svg += '<line x1="' + x + '" y1="54" x2="' + x + '" y2="' + (height - 10) + '" class="seq-lifeline"></line>';
  });
  messages.forEach((m, i) => {
    const y = startY + i * rowGap;
    const x1 = laneX[m.from], x2 = laneX[m.to];
    const mid = (x1 + x2) / 2;
    const cls = "seq-msg" + (m.tone ? " seq-msg-" + m.tone : "");
    svg += '<line x1="' + x1 + '" y1="' + y + '" x2="' + x2 + '" y2="' + y + '" class="' + cls + '" marker-end="url(#' + arrowId + ')"></line>';
    svg += '<text x="' + mid + '" y="' + (y - 6) + '" text-anchor="middle" class="seq-msg-label">' + escapeHtml(m.label) + '</text>';
  });
  svg += '</svg>';
  container.innerHTML = svg;
}
</script>
<header>
  <div class="header-top">
    <h1>JFrog Artifactory repository audit</h1>
    __HEADER_INSTANCE__
  </div>
  <p>Generated by jfps from <code>__SOURCE__</code> - __ROW_COUNT__ repositories. Click any
  finding card to filter the explorer below; click a row for full details.</p>
</header>
<main>
  <nav class="report-tabs" id="report-tabs">__NAV_TABS__</nav>

  <section class="report-panel__REPOS_ACTIVE_CLASS__" data-report-panel="repos">
  <h2>Repository estate summary</h2>
  <section class="stats">__STAT_CARDS__
  </section>

  <h2>Findings</h2>
  <section class="findings" id="findings">__FINDING_CARDS__
  </section>

  <section class="explorer" id="explorer">
    <div class="toolbar" id="tabs">
      <input id="search" type="search" placeholder="Search repository name, type, finding...">
      <select id="package"><option value="">All package types</option></select>
      <button class="clear" id="clear">Clear filters</button>
    </div>
    <div class="active-filter" id="active-filter"></div>
    <div class="table-wrap">
      <table>
        <thead id="thead"></thead>
        <tbody id="tbody"></tbody>
      </table>
    </div>
    <section class="drawer" id="drawer"></section>
  </section>
  </section>

  __EXTRA_PANELS__

  <p class="foot">Generated __GENERATED_AT__ from __SOURCE__. All data is embedded in this HTML - nothing is fetched at view time.</p>
</main>
<script>
const DATA = __JSON_ROWS__;
const FINDING_DEFS = __JSON_FINDINGS__;

let repoType = "";
let packageValue = "";
let finding = "";
let explanation = "";

const search = document.querySelector("#search");
const packageSelect = document.querySelector("#package");
const tbody = document.querySelector("#tbody");
const thead = document.querySelector("#thead");
const activeFilter = document.querySelector("#active-filter");
const drawer = document.querySelector("#drawer");
const tabsBar = document.querySelector("#tabs");

const CORE_COLUMNS = [
  ["repo_name", "Repository"], ["package_type", "Package"], ["repo_type", "Type"],
  ["files_count", "Files"], ["usage_gb", "Usage GB"], ["last_activity_date", "Last activity"],
  ["cleanup_policy_keys", "Cleanup policy"],
];

function buildTabs() {
  const types = [...new Set(DATA.map(r => r.repo_type).filter(Boolean))].sort();
  const bar = document.createElement("div");
  bar.style.display = "flex";
  bar.style.gap = "8px";
  bar.style.flexWrap = "wrap";
  const allTab = mkTab("", "All (" + DATA.length + ")");
  bar.appendChild(allTab);
  types.forEach(t => {
    const count = DATA.filter(r => r.repo_type === t).length;
    bar.appendChild(mkTab(t, t + " (" + count + ")"));
  });
  tabsBar.prepend(bar);
}

function mkTab(value, label) {
  const btn = document.createElement("button");
  btn.className = "tab" + (value === repoType ? " active" : "");
  btn.textContent = label;
  btn.dataset.repoType = value;
  btn.addEventListener("click", () => {
    repoType = value;
    document.querySelectorAll(".tab").forEach(b => b.classList.toggle("active", b.dataset.repoType === value));
    rebuildPackageOptions();
    render();
  });
  return btn;
}

function rebuildPackageOptions() {
  const scoped = repoType ? DATA.filter(r => r.repo_type === repoType) : DATA;
  const values = [...new Set(scoped.map(r => r.package_type).filter(Boolean))].sort();
  packageSelect.innerHTML = '<option value="">All package types</option>' +
    values.map(v => "<option " + (v === packageValue ? "selected" : "") + ">" + escapeHtml(v) + "</option>").join("");
}

function renderBadges(findings) {
  return (findings || []).map(f =>
    '<span class="badge ' + escapeHtml(f) + '">' + escapeHtml(f) + "</span>").join("");
}

function visibleRows() {
  const query = search.value.trim().toLowerCase();
  return DATA.filter(r => {
    if (repoType && r.repo_type !== repoType) return false;
    if (packageValue && r.package_type !== packageValue) return false;
    if (finding && !(r.findings || []).includes(finding)) return false;
    if (query && !JSON.stringify(r).toLowerCase().includes(query)) return false;
    return true;
  });
}

function render() {
  const rows = visibleRows();
  const cols = CORE_COLUMNS.filter(([key]) => DATA.length === 0 || key in DATA[0]);

  thead.innerHTML = "<tr>" + cols.map(([, label]) =>
    "<th" + (label === "Files" || label === "Usage GB" ? ' class="number"' : "") + ">" + label + "</th>").join("") +
    "<th>Findings</th></tr>";

  tbody.innerHTML = rows.map((r, index) => {
    const cells = cols.map(([key]) => {
      const value = r[key];
      const numeric = key === "files_count" || key === "usage_gb";
      const display = key === "usage_gb" ? Number(value).toLocaleString(undefined, {maximumFractionDigits: 4})
        : numeric ? Number(value).toLocaleString() : escapeHtml(value || "");
      return "<td" + (numeric ? ' class="number"' : "") + ">" + display + "</td>";
    }).join("");
    return '<tr data-index="' + index + '">' + cells + "<td>" + renderBadges(r.findings) + "</td></tr>";
  }).join("");

  tbody.querySelectorAll("tr").forEach(row =>
    row.addEventListener("click", () => showDetails(rows[Number(row.dataset.index)])));

  const label = finding ?
    rows.length.toLocaleString() + " repos - filter: " + finding + ". " + explanation :
    rows.length.toLocaleString() + " repositories shown";
  activeFilter.textContent = label;
  activeFilter.classList.add("show");
}

function showDetails(row) {
  const pairs = Object.entries(row).filter(([key]) => key !== "findings");
  const detailRows = pairs.map(([key, value]) =>
    "<div><dt>" + escapeHtml(prettyLabel(key)) + "</dt><dd>" +
    escapeHtml(value === "" || value === undefined ? "None" : value) + "</dd></div>").join("");
  drawer.innerHTML = "<h3>" + escapeHtml(row.repo_name) + '</h3><dl class="drawer-grid">' + detailRows +
    "<div><dt>Findings</dt><dd>" + (renderBadges(row.findings) || "None") + "</dd></div></dl>";
  drawer.classList.add("show");
  drawer.scrollIntoView({behavior: "smooth", block: "nearest"});
}

document.querySelectorAll("#findings .finding").forEach(card =>
  card.addEventListener("click", () => {
    const id = card.dataset.finding;
    const isActive = finding === id;
    finding = isActive ? "" : id;
    explanation = isActive ? "" : card.dataset.explain;
    document.querySelectorAll("#findings .finding").forEach(c => c.classList.toggle("active", c.dataset.finding === finding));
    document.querySelector("#explorer").scrollIntoView({behavior: "smooth", block: "start"});
    render();
  }));

search.addEventListener("input", render);
packageSelect.addEventListener("change", () => { packageValue = packageSelect.value; render(); });
document.querySelector("#clear").addEventListener("click", () => {
  repoType = ""; packageValue = ""; finding = ""; explanation = "";
  search.value = ""; drawer.classList.remove("show");
  document.querySelectorAll(".tab").forEach(b => b.classList.toggle("active", b.dataset.repoType === ""));
  document.querySelectorAll("#findings .finding").forEach(c => c.classList.remove("active"));
  rebuildPackageOptions();
  render();
});

buildTabs();
rebuildPackageOptions();
render();
initWavePlanning();
initMigrationWorkflow();

// --- report-level tab switching (Repos / Overview / Access / Projects / Federation & Replication) ---

function setReportPanel(id) {
  document.querySelectorAll("[data-report-tab]").forEach(b => b.classList.toggle("active", b.dataset.reportTab === id));
  document.querySelectorAll("[data-report-panel]").forEach(p => p.classList.toggle("active", p.dataset.reportPanel === id));
}
document.querySelectorAll("[data-report-tab]").forEach(b =>
  b.addEventListener("click", () => setReportPanel(b.dataset.reportTab)));

</script>
</body>
</html>
`
