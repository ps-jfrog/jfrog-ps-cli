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
header h1 { margin:0 0 7px; font-size:25px; }
header p { margin:0; color:#c5cfdd; max-width:950px; }
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
@media (max-width:700px) { header,main { padding-left:18px; padding-right:18px; } }
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
  if (Array.isArray(value)) return value.join(", ");
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
</script>
<header>
  <h1>JFrog Artifactory repository audit</h1>
  <p>Generated by jfps from <code>__SOURCE__</code> - __ROW_COUNT__ repositories. Click any
  finding card to filter the explorer below; click a row for full details.</p>
</header>
<main>
  <nav class="report-tabs" id="report-tabs">__NAV_TABS__</nav>

  <section class="report-panel active" data-report-panel="repos">
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
