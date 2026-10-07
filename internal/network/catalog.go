package network

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// FlowSide 区分一次网的供水侧与回水侧。
type FlowSide string

const (
	SideSupply FlowSide = "supply"
	SideReturn FlowSide = "return"
)

// AssetKind 标识目录中资产的设备类别。
type AssetKind string

const (
	KindZone      AssetKind = "zone"
	KindStation   AssetKind = "station"
	KindExchanger AssetKind = "heat_exchanger"
	KindPump      AssetKind = "pump"
	KindValve     AssetKind = "valve"
	KindRelief    AssetKind = "pressure_relief"
	KindBranch    AssetKind = "branch"
	KindEndpoint  AssetKind = "endpoint"
)

// ZoneAsset 描述一个供热分区。
type ZoneAsset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// StationAsset 描述一座换热站，归属于某个分区。
type StationAsset struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	ZoneID string `json:"zone_id"`
}

// ExchangerAsset 描述站内的单台换热器。
type ExchangerAsset struct {
	ID        string `json:"id"`
	StationID string `json:"station_id"`
}

// PumpAsset 描述循环泵，绑定到其服务的分区与站点（站点可为空表示分区级主泵）。
type PumpAsset struct {
	ID        string `json:"id"`
	ZoneID    string `json:"zone_id"`
	StationID string `json:"station_id,omitempty"`
}

// ValveRole 描述阀门在拓扑中的安全角色。
type ValveRole string

const (
	ValveBoundary ValveRole = "boundary" // 边界隔离阀
	ValveBypass   ValveRole = "bypass"   // 联锁旁通阀
	ValveManual   ValveRole = "manual"   // 站内手动调节阀
)

// ValveAsset 描述一台可操作阀门。
type ValveAsset struct {
	ID        string    `json:"id"`
	Role      ValveRole `json:"role"`
	ZoneID    string    `json:"zone_id"`
	StationID string    `json:"station_id,omitempty"`
}

// ReliefAsset 描述边界泄压（安全泄放）设备。
type ReliefAsset struct {
	ID       string `json:"id"`
	ZoneID   string `json:"zone_id"`
	BranchID string `json:"branch_id"`
}

// EndpointAsset 描述分支的一个供/回水端点。
type EndpointAsset struct {
	ID       string   `json:"id"`
	BranchID string   `json:"branch_id"`
	Side     FlowSide `json:"side"`
}

// BranchAsset 描述一条供回水分支：一个供水端点、一个回水端点，
// 至少两台边界隔离阀与一台泄压设备共同构成安全隔离能力。
type BranchAsset struct {
	ID             string   `json:"id"`
	ZoneID         string   `json:"zone_id"`
	StationID      string   `json:"station_id,omitempty"`
	SupplyEndpoint string   `json:"supply_endpoint"`
	ReturnEndpoint string   `json:"return_endpoint"`
	BoundaryValves []string `json:"boundary_valves"`
	ReliefValve    string   `json:"relief_valve"`
}

// Catalog 是供热网络资产目录。所有运行页面（恢复、联锁、调度、运维）
// 共享同一个 Catalog 指针与同一个 Version，目录在启动时校验，运行期只读。
type Catalog struct {
	Zones      []ZoneAsset      `json:"zones"`
	Stations   []StationAsset   `json:"stations"`
	Exchangers []ExchangerAsset `json:"exchangers"`
	Pumps      []PumpAsset      `json:"pumps"`
	Valves     []ValveAsset     `json:"valves"`
	Reliefs    []ReliefAsset    `json:"reliefs"`
	Endpoints  []EndpointAsset  `json:"endpoints"`
	Branches   []BranchAsset    `json:"branches"`

	version string
}

// NewCatalog 规范化并构建资产目录。资产顺序被重排为确定顺序，
// 这样相同的资产集合总是得到相同的版本号。
func NewCatalog(zones []ZoneAsset, stations []StationAsset, exchangers []ExchangerAsset, pumps []PumpAsset,
	valves []ValveAsset, reliefs []ReliefAsset, endpoints []EndpointAsset, branches []BranchAsset) *Catalog {
	catalog := &Catalog{
		Zones: append([]ZoneAsset(nil), zones...), Stations: append([]StationAsset(nil), stations...),
		Exchangers: append([]ExchangerAsset(nil), exchangers...), Pumps: append([]PumpAsset(nil), pumps...),
		Valves: append([]ValveAsset(nil), valves...), Reliefs: append([]ReliefAsset(nil), reliefs...),
		Endpoints: append([]EndpointAsset(nil), endpoints...),
	}
	for _, branch := range branches {
		branch.BoundaryValves = append([]string(nil), branch.BoundaryValves...)
		sort.Strings(branch.BoundaryValves)
		catalog.Branches = append(catalog.Branches, branch)
	}
	sort.Slice(catalog.Zones, func(i, j int) bool { return catalog.Zones[i].ID < catalog.Zones[j].ID })
	sort.Slice(catalog.Stations, func(i, j int) bool { return catalog.Stations[i].ID < catalog.Stations[j].ID })
	sort.Slice(catalog.Exchangers, func(i, j int) bool {
		if catalog.Exchangers[i].StationID != catalog.Exchangers[j].StationID {
			return catalog.Exchangers[i].StationID < catalog.Exchangers[j].StationID
		}
		return catalog.Exchangers[i].ID < catalog.Exchangers[j].ID
	})
	sort.Slice(catalog.Pumps, func(i, j int) bool { return catalog.Pumps[i].ID < catalog.Pumps[j].ID })
	sort.Slice(catalog.Valves, func(i, j int) bool { return catalog.Valves[i].ID < catalog.Valves[j].ID })
	sort.Slice(catalog.Reliefs, func(i, j int) bool { return catalog.Reliefs[i].ID < catalog.Reliefs[j].ID })
	sort.Slice(catalog.Endpoints, func(i, j int) bool { return catalog.Endpoints[i].ID < catalog.Endpoints[j].ID })
	sort.Slice(catalog.Branches, func(i, j int) bool { return catalog.Branches[i].ID < catalog.Branches[j].ID })
	catalog.version = catalog.computeVersion()
	return catalog
}

// Version 返回目录的内容版本（短哈希），供所有页面与响应引用。
func (c *Catalog) Version() string {
	return c.version
}

func (c *Catalog) computeVersion() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "zones=%d stations=%d exchangers=%d pumps=%d valves=%d reliefs=%d endpoints=%d branches=%d\n",
		len(c.Zones), len(c.Stations), len(c.Exchangers), len(c.Pumps), len(c.Valves), len(c.Reliefs), len(c.Endpoints), len(c.Branches))
	for _, zone := range c.Zones {
		fmt.Fprintf(&builder, "z:%s:%s\n", zone.ID, zone.Name)
	}
	for _, station := range c.Stations {
		fmt.Fprintf(&builder, "s:%s:%s:%s\n", station.ID, station.ZoneID, station.Name)
	}
	for _, exchanger := range c.Exchangers {
		fmt.Fprintf(&builder, "h:%s:%s\n", exchanger.ID, exchanger.StationID)
	}
	for _, pump := range c.Pumps {
		fmt.Fprintf(&builder, "p:%s:%s:%s\n", pump.ID, pump.ZoneID, pump.StationID)
	}
	for _, valve := range c.Valves {
		fmt.Fprintf(&builder, "v:%s:%s:%s:%s\n", valve.ID, valve.Role, valve.ZoneID, valve.StationID)
	}
	for _, relief := range c.Reliefs {
		fmt.Fprintf(&builder, "r:%s:%s:%s\n", relief.ID, relief.ZoneID, relief.BranchID)
	}
	for _, endpoint := range c.Endpoints {
		fmt.Fprintf(&builder, "e:%s:%s:%s\n", endpoint.ID, endpoint.BranchID, endpoint.Side)
	}
	for _, branch := range c.Branches {
		fmt.Fprintf(&builder, "b:%s:%s:%s:%s:%s:%s:%s\n", branch.ID, branch.ZoneID, branch.StationID,
			branch.SupplyEndpoint, branch.ReturnEndpoint, strings.Join(branch.BoundaryValves, ","), branch.ReliefValve)
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return "cat-" + hex.EncodeToString(sum[:])[:12]
}

// Snapshot 是目录的可序列化视图，携带版本号。
type CatalogSnapshot struct {
	Version    string           `json:"version"`
	Zones      []ZoneAsset      `json:"zones"`
	Stations   []StationAsset   `json:"stations"`
	Exchangers []ExchangerAsset `json:"exchangers"`
	Pumps      []PumpAsset      `json:"pumps"`
	Valves     []ValveAsset     `json:"valves"`
	Reliefs    []ReliefAsset    `json:"reliefs"`
	Endpoints  []EndpointAsset  `json:"endpoints"`
	Branches   []BranchAsset    `json:"branches"`
}

// Snapshot 返回供 API 与页面使用的只读目录视图。
func (c *Catalog) Snapshot() CatalogSnapshot {
	return CatalogSnapshot{
		Version: c.version, Zones: append([]ZoneAsset(nil), c.Zones...),
		Stations: append([]StationAsset(nil), c.Stations...), Exchangers: append([]ExchangerAsset(nil), c.Exchangers...),
		Pumps: append([]PumpAsset(nil), c.Pumps...), Valves: append([]ValveAsset(nil), c.Valves...),
		Reliefs: append([]ReliefAsset(nil), c.Reliefs...), Endpoints: append([]EndpointAsset(nil), c.Endpoints...),
		Branches: append([]BranchAsset(nil), c.Branches...),
	}
}
