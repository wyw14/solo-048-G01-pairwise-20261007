const page = document.body.dataset.page;
const state = { overview: null };

function text(value) {
  return value === undefined || value === null ? "-" : String(value);
}

function panel(title, rows) {
  const body = rows.map(([name, value]) => `<dt>${name}</dt><dd>${text(value)}</dd>`).join("");
  return `<article class="panel"><h3>${title}</h3><dl>${body}</dl></article>`;
}

function metrics(data) {
  const active = data.zones.filter((zone) => zone.phase === "active").length;
  const online = data.stations.filter((station) => station.online).length;
  const incidents = data.incidents.filter((incident) => incident.active).length;
  const published = data.plans.filter((plan) => plan.status === "published").length;
  return [["运行区域", active], ["在线换热站", online], ["活动事故", incidents], ["已发布计划", published]]
    .map(([name, value]) => `<div class="metric"><span>${name}</span><strong>${value}</strong></div>`).join("");
}

function branchPanel(catalog, branch) {
  const valves = (branch.boundary_valves || []).join(", ");
  const rows = [
    ["分区", branch.zone_id],
    ["供水端点", branch.supply_endpoint],
    ["回水端点", branch.return_endpoint],
    ["边界隔离阀", valves],
    ["泄压设备", branch.relief_valve],
  ];
  if (branch.station_id) rows.splice(1, 0, ["换热站", branch.station_id]);
  return panel(branch.id, rows);
}

// catalogPanel 渲染目录内容；四个页面共享同一份 catalog 数据与版本号。
function catalogPanel(data) {
  const catalog = data.catalog;
  if (!catalog) return "";
  if (page === "network") {
    return (catalog.branches || []).map((branch) => branchPanel(catalog, branch)).join("");
  }
  if (page === "stations") {
    return (catalog.stations || []).map((station) => panel(station.name || station.id, [
      ["站点编号", station.id],
      ["所属分区", station.zone_id],
      ["换热器", (catalog.exchangers || []).filter((item) => item.station_id === station.id).map((item) => item.id).join(", ")],
      ["循环泵", (catalog.pumps || []).filter((item) => item.station_id === station.id).map((item) => item.id).join(", ") || "-"],
    ])).join("");
  }
  if (page === "dispatch") {
    return panel("目录版本校验", [
      ["目录版本", catalog.version],
      ["分区数", (catalog.zones || []).length],
      ["站点数", (catalog.stations || []).length],
      ["分支数", (catalog.branches || []).length],
      ["全部计划均带 catalog_version", data.plans.every((plan) => plan.catalog_version === catalog.version) ? "是" : "否"],
    ]);
  }
  return panel("隔离与泄压器件", [
    ["目录版本", catalog.version],
    ["边界阀门", (catalog.valves || []).filter((item) => item.role === "boundary").length],
    ["泄压设备", (catalog.reliefs || []).length],
    ["每条分支", "供/回水端点 + 2 台边界阀 + 1 台泄压"],
  ]);
}

function pageContent(data) {
  if (page === "network") return data.zones.map((zone) => panel(zone.name, [["阶段", zone.phase], ["泵级", zone.pump_stage], ["旁通", zone.bypass_open ? "开启" : "关闭"], ["压差", (zone.supply_pressure - zone.return_pressure).toFixed(2) + " MPa"]])).join("");
  if (page === "stations") return data.stations.map((station) => panel(station.name, [["区域", station.zone_id], ["通信", station.online ? "在线" : "离线"], ["目标", station.target_celsius + " °C"], ["控制代际", station.controller_epoch], ["人工保持", station.hold ? "是" : "否"]])).join("");
  if (page === "incidents") return data.incidents.length ? data.incidents.map((incident) => panel(incident.kind, [["站点", incident.station_id], ["等级", incident.severity], ["状态", incident.active ? "处理中" : "已解除"], ["说明", incident.message]])).join("") : panel("当前无活动事故", [["隔离会话", data.isolations.length], ["系统状态", "稳定"]]);
  return data.plans.length ? data.plans.map((plan) => panel(plan.id.slice(0, 8), [["状态", plan.status], ["目录版本", plan.catalog_version], ["来源 revision", plan.source_revision], ["站点数", Object.keys(plan.targets || {}).length], ["说明", plan.message || "-"]])).join("") : panel("等待调度计划", [["当前 revision", Math.max(...data.stations.map((station) => station.target_revision))], ["人工保持", data.stations.filter((station) => station.hold).length]]);
}

async function load() {
  const connection = document.querySelector("#connection");
  try {
    const response = await fetch("/api/v1/overview", { cache: "no-store" });
    const payload = await response.json();
    if (!response.ok) throw new Error(payload.error || "request failed");
    state.overview = payload.data;
    document.querySelector("#summary").innerHTML = metrics(payload.data);
    document.querySelector("#content").innerHTML = pageContent(payload.data);
    document.querySelector("#catalog-version").textContent = "目录 " + payload.data.catalog_version;
    const catalogPanelElement = document.querySelector("#catalog-panel");
    const catalogMarkup = catalogPanel(payload.data);
    document.querySelector("#catalog-content").innerHTML = catalogMarkup;
    catalogPanelElement.hidden = !catalogMarkup;
    connection.textContent = "实时连接";
  } catch (error) {
    connection.textContent = "连接异常";
    document.querySelector("#content").innerHTML = panel("无法加载状态", [["错误", error.message]]);
  }
}

document.querySelector("#refresh").addEventListener("click", load);
load();
