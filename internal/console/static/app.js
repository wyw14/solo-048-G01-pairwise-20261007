const page = document.body.dataset.page;
const state = { overview: null, catalog: null };

function text(value) {
  return value === undefined || value === null ? "-" : String(value);
}

function shortVersion(version) {
  if (!version || !version.digest) return "-";
  return `v${version.schema}-${version.digest.slice(0, 7)}`;
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

function catalogPanels(catalog) {
  const nodeName = (id) => {
    const node = catalog.nodes.find((item) => item.id === id);
    return node ? `${node.name}（${node.conduit === "supply" ? "供水" : "回水"}）` : id;
  };
  const stationName = (id) => {
    const station = catalog.stations.find((item) => item.id === id);
    return station ? station.name : id;
  };
  const branchPanels = catalog.branches.map((branch) => {
    const zone = catalog.zones.find((item) => item.id === branch.zone_id);
    return panel(`${branch.name} <small>${branch.id}</small>`, [
      ["所属分区", zone ? zone.name : branch.zone_id],
      ["供水端点", nodeName(branch.supply_node)],
      ["回水端点", nodeName(branch.return_node)],
      ["沿线站点", branch.stations.map(stationName).join("、")],
      ["供水边界阀", branch.supply_boundary],
      ["回水边界阀", branch.return_boundary],
      ["泄压设备", branch.relief],
    ]);
  }).join("");
  const stationPanels = catalog.stations.map((station) => panel(`${station.name} <small>${station.id}</small>`, [
    ["所属分区/分支", `${station.zone_id} / ${station.branch_id}`],
    ["恢复必备", station.required ? "是" : "否"],
    ["换热器", station.exchangers.join("、")],
    ["阀门", station.valves.join("、")],
  ])).join("");
  return `<div class="span-all"><h3 class="group-title">分区与分支（供回水端点 / 安全隔离器件）</h3></div>${branchPanels}
    <div class="span-all"><h3 class="group-title">站点资产（换热器 / 阀门）</h3></div>${stationPanels}`;
}

function pageContent(data, catalog) {
  if (page === "assets") return catalogPanels(catalog);
  if (page === "network") {
    return data.zones.map((zone) => {
      const candidate = data.interlocks[zone.id] || {};
      const pumps = (catalog.zones.find((item) => item.id === zone.id) || { pumps: [] }).pumps;
      return panel(zone.name, [
        ["阶段", zone.phase], ["循环泵", pumps.join("、")],
        ["泵级", zone.pump_stage], ["旁通", zone.bypass_open ? "开启" : "关闭"],
        ["压差", (zone.supply_pressure - zone.return_pressure).toFixed(2) + " MPa"],
        ["联锁引用目录", shortVersion(candidate.catalog_version)],
      ]);
    }).join("");
  }
  if (page === "stations") {
    return data.stations.map((station) => panel(station.name, [
      ["区域", station.zone_id], ["通信", station.online ? "在线" : "离线"],
      ["目标", station.target_celsius + " °C"], ["控制代际", station.controller_epoch],
      ["检修", station.maintenance ? "检修中（保持）" : "正常"], ["人工保持", station.hold ? "是" : "否"],
      ["运维引用目录", shortVersion(catalog.version)],
    ])).join("");
  }
  if (page === "incidents") {
    const isolationPanels = data.isolations.length ? data.isolations.map((session) => panel(`隔离会话 ${session.id.slice(0, 8)}`, [
      ["分支", session.section_id], ["分区", session.zone_id], ["状态", session.status],
      ["泄压", session.relief.valve_id || "-"], ["边界阀", (session.boundaries || []).map((proof) => proof.valve_id).join("、") || "-"],
    ])).join("") : "";
    const incidentPanels = data.incidents.length ? data.incidents.map((incident) => panel(incident.kind, [
      ["站点", incident.station_id], ["等级", incident.severity],
      ["状态", incident.active ? "处理中" : "已解除"], ["说明", incident.message],
    ])).join("") : panel("当前无活动事故", [["隔离会话", data.isolations.length], ["系统状态", "稳定"]]);
    return `${panel("恢复 / 隔离基准", [["引用目录版本", shortVersion(catalog.version)], ["分区数", catalog.zones.length], ["可隔离分支数", catalog.branches.length]])}${incidentPanels}${isolationPanels}`;
  }
  // dispatch
  return data.plans.length ? data.plans.map((plan) => {
    const stale = plan.catalog_version && plan.catalog_version.digest !== catalog.version.digest;
    return panel(plan.id.slice(0, 8), [
      ["状态", plan.status], ["来源 revision", plan.source_revision],
      ["站点数", Object.keys(plan.targets || {}).length], ["说明", plan.message || "-"],
      ["计划目录版本", shortVersion(plan.catalog_version)],
      ["当前目录版本", stale ? `${shortVersion(catalog.version)}（已过期，禁止发布）` : shortVersion(catalog.version)],
    ]);
  }).join("") : panel("等待调度计划", [
    ["当前 revision", Math.max(...data.stations.map((station) => station.target_revision))],
    ["人工保持", data.stations.filter((station) => station.hold).length],
    ["调度引用目录", shortVersion(catalog.version)],
  ]);
}

async function load() {
  const connection = document.querySelector("#connection");
  const catalogChip = document.querySelector("#catalog");
  try {
    const [catalogResponse, overviewResponse] = await Promise.all([
      fetch("/api/v1/catalog", { cache: "no-store" }),
      fetch("/api/v1/overview", { cache: "no-store" }),
    ]);
    const catalogPayload = await catalogResponse.json();
    const overviewPayload = await overviewResponse.json();
    if (!catalogResponse.ok) throw new Error(catalogPayload.error || "catalog request failed");
    if (!overviewResponse.ok) throw new Error(overviewPayload.error || "overview request failed");
    state.catalog = catalogPayload.data;
    state.overview = overviewPayload.data;
    // 所有页面共用同一个目录版本；若总览携带版本与目录端点不一致则显式告警。
    const versionsMatch = state.overview.catalog_version
      && state.overview.catalog_version.digest === state.catalog.version.digest
      && state.overview.catalog_version.schema === state.catalog.version.schema;
    catalogChip.textContent = versionsMatch ? `资产目录 ${shortVersion(state.catalog.version)}` : "目录版本不一致！";
    catalogChip.classList.toggle("mismatch", !versionsMatch);
    document.querySelector("#section-heading").textContent = page === "assets" ? "资产拓扑（分区 / 分支 / 站点 / 设备）" : "实时状态";
    document.querySelector("#summary").innerHTML = page === "assets" ? "" : metrics(state.overview);
    document.querySelector("#content").innerHTML = pageContent(state.overview, state.catalog);
    connection.textContent = "实时连接";
  } catch (error) {
    connection.textContent = "连接异常";
    catalogChip.textContent = "目录不可用";
    document.querySelector("#content").innerHTML = panel("无法加载状态", [["错误", error.message]]);
  }
}

document.querySelector("#refresh").addEventListener("click", load);
load();
