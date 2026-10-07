package network

import (
	"fmt"
	"sort"
)

// ValidationIssue 描述一条目录校验问题。
type ValidationIssue struct {
	Kind    string `json:"kind"`
	AssetID string `json:"asset_id"`
	Message string `json:"message"`
}

func (i ValidationIssue) Error() string {
	return fmt.Sprintf("catalog %s %s: %s", i.Kind, i.AssetID, i.Message)
}

// Validate 在启动时校验目录：
//  1. 资产 ID 非空且全局唯一；
//  2. 分区/站点/换热器/泵/阀门/泄压/端点之间的归属引用完整；
//  3. 每条分支恰好有一个供水端点和一个回水端点，且端点属于该分支；
//  4. 每条分支至少配置两台边界隔离阀并引用一台泄压设备（安全隔离器件齐备）。
func (c *Catalog) Validate() error {
	issues := c.ValidationIssues()
	if len(issues) == 0 {
		return nil
	}
	return issues[0]
}

// ValidationIssues 返回全部校验问题，便于启动日志一次性暴露所有配置错误。
func (c *Catalog) ValidationIssues() []ValidationIssue {
	var issues []ValidationIssue
	seen := make(map[string]string)
	note := func(kind, id, message string) {
		issues = append(issues, ValidationIssue{Kind: kind, AssetID: id, Message: message})
	}
	claim := func(kind, id string) bool {
		if id == "" {
			issues = append(issues, ValidationIssue{Kind: kind, AssetID: id, Message: "missing id"})
			return false
		}
		if previous, ok := seen[id]; ok {
			issues = append(issues, ValidationIssue{Kind: kind, AssetID: id, Message: "duplicate id, also declared as " + previous})
			return false
		}
		seen[id] = kind
		return true
	}

	for _, zone := range c.Zones {
		claim(string(KindZone), zone.ID)
		if zone.Name == "" {
			note(string(KindZone), zone.ID, "missing name")
		}
	}
	for _, station := range c.Stations {
		claim(string(KindStation), station.ID)
		if station.Name == "" {
			note(string(KindStation), station.ID, "missing name")
		}
		if _, ok := zoneIn(c, station.ZoneID); !ok {
			note(string(KindStation), station.ID, "references unknown zone "+station.ZoneID)
		}
	}
	for _, exchanger := range c.Exchangers {
		claim(string(KindExchanger), exchanger.ID)
		if station, ok := findStation(c, exchanger.StationID); !ok {
			note(string(KindExchanger), exchanger.ID, "references unknown station "+exchanger.StationID)
		} else if _, ok := zoneIn(c, station.ZoneID); !ok {
			note(string(KindExchanger), exchanger.ID, "station "+station.ID+" references unknown zone "+station.ZoneID)
		}
	}
	for _, pump := range c.Pumps {
		claim(string(KindPump), pump.ID)
		if _, ok := zoneIn(c, pump.ZoneID); !ok {
			note(string(KindPump), pump.ID, "references unknown zone "+pump.ZoneID)
		}
		if pump.StationID != "" {
			if station, ok := findStation(c, pump.StationID); !ok {
				note(string(KindPump), pump.ID, "references unknown station "+pump.StationID)
			} else if station.ZoneID != pump.ZoneID {
				note(string(KindPump), pump.ID, fmt.Sprintf("station %s belongs to zone %s, pump declares %s", station.ID, station.ZoneID, pump.ZoneID))
			}
		}
	}
	for _, valve := range c.Valves {
		claim(string(KindValve), valve.ID)
		switch valve.Role {
		case ValveBoundary, ValveBypass, ValveManual:
		default:
			note(string(KindValve), valve.ID, "unknown valve role "+string(valve.Role))
		}
		if _, ok := zoneIn(c, valve.ZoneID); !ok {
			note(string(KindValve), valve.ID, "references unknown zone "+valve.ZoneID)
		}
		if valve.StationID != "" {
			if station, ok := findStation(c, valve.StationID); !ok {
				note(string(KindValve), valve.ID, "references unknown station "+valve.StationID)
			} else if station.ZoneID != valve.ZoneID {
				note(string(KindValve), valve.ID, fmt.Sprintf("station %s belongs to zone %s, valve declares %s", station.ID, station.ZoneID, valve.ZoneID))
			}
		}
	}
	for _, relief := range c.Reliefs {
		claim(string(KindRelief), relief.ID)
		if _, ok := zoneIn(c, relief.ZoneID); !ok {
			note(string(KindRelief), relief.ID, "references unknown zone "+relief.ZoneID)
		}
		if branch, ok := findBranch(c, relief.BranchID); !ok {
			note(string(KindRelief), relief.ID, "references unknown branch "+relief.BranchID)
		} else if branch.ZoneID != relief.ZoneID {
			note(string(KindRelief), relief.ID, fmt.Sprintf("branch %s belongs to zone %s, relief declares %s", branch.ID, branch.ZoneID, relief.ZoneID))
		}
	}
	for _, endpoint := range c.Endpoints {
		claim(string(KindEndpoint), endpoint.ID)
		switch endpoint.Side {
		case SideSupply, SideReturn:
		default:
			note(string(KindEndpoint), endpoint.ID, "endpoint side must be supply or return")
		}
		if _, ok := findBranch(c, endpoint.BranchID); !ok {
			note(string(KindEndpoint), endpoint.ID, "references unknown branch "+endpoint.BranchID)
		}
	}
	for _, branch := range c.Branches {
		claim(string(KindBranch), branch.ID)
		if _, ok := zoneIn(c, branch.ZoneID); !ok {
			note(string(KindBranch), branch.ID, "references unknown zone "+branch.ZoneID)
		}
		if branch.StationID != "" {
			if station, ok := findStation(c, branch.StationID); !ok {
				note(string(KindBranch), branch.ID, "references unknown station "+branch.StationID)
			} else if station.ZoneID != branch.ZoneID {
				note(string(KindBranch), branch.ID, fmt.Sprintf("station %s belongs to zone %s, branch declares %s", station.ID, station.ZoneID, branch.ZoneID))
			}
		}
		// 供回水端点必须成对且确实属于本分支。
		supply, err := endpointFor(c, branch.SupplyEndpoint, SideSupply, branch.ID)
		if err != nil {
			issues = append(issues, validationFrom(KindBranch, branch.ID, err))
		}
		back, err := endpointFor(c, branch.ReturnEndpoint, SideReturn, branch.ID)
		if err != nil {
			issues = append(issues, validationFrom(KindBranch, branch.ID, err))
		}
		if branch.SupplyEndpoint != "" && branch.SupplyEndpoint == branch.ReturnEndpoint {
			note(string(KindBranch), branch.ID, "supply and return endpoints must differ")
		}
		if supply.ID != "" && back.ID != "" && supply.ID == back.ID {
			note(string(KindBranch), branch.ID, "supply and return endpoints resolve to the same endpoint")
		}
		// 安全隔离器件：至少两台边界隔离阀 + 一台泄压设备。
		if len(branch.BoundaryValves) < 2 {
			note(string(KindBranch), branch.ID, "requires at least two boundary isolation valves")
		}
		valveSeen := make(map[string]bool)
		for _, valveID := range branch.BoundaryValves {
			if valveSeen[valveID] {
				note(string(KindBranch), branch.ID, "boundary valve listed twice: "+valveID)
			}
			valveSeen[valveID] = true
			valve, ok := findValve(c, valveID)
			if !ok {
				note(string(KindBranch), branch.ID, "references unknown boundary valve "+valveID)
				continue
			}
			if valve.Role != ValveBoundary {
				note(string(KindBranch), branch.ID, "valve "+valveID+" has role "+string(valve.Role)+", expected boundary")
			}
			if valve.ZoneID != branch.ZoneID {
				note(string(KindBranch), branch.ID, fmt.Sprintf("boundary valve %s belongs to zone %s, branch declares %s", valveID, valve.ZoneID, branch.ZoneID))
			}
		}
		relief, ok := findRelief(c, branch.ReliefValve)
		if !ok {
			note(string(KindBranch), branch.ID, "references unknown relief device "+branch.ReliefValve)
		} else {
			if relief.BranchID != branch.ID {
				note(string(KindBranch), branch.ID, fmt.Sprintf("relief %s is bound to branch %s", branch.ReliefValve, relief.BranchID))
			}
			if relief.ZoneID != branch.ZoneID {
				note(string(KindBranch), branch.ID, fmt.Sprintf("relief %s belongs to zone %s, branch declares %s", branch.ReliefValve, relief.ZoneID, branch.ZoneID))
			}
		}
	}
	return issues
}

func endpointFor(c *Catalog, endpointID string, side FlowSide, branchID string) (EndpointAsset, error) {
	if endpointID == "" {
		return EndpointAsset{}, fmt.Errorf("%s endpoint is empty", side)
	}
	endpoint, ok := findEndpoint(c, endpointID)
	if !ok {
		return EndpointAsset{}, fmt.Errorf("%s endpoint %s not found", side, endpointID)
	}
	if endpoint.Side != side {
		return EndpointAsset{}, fmt.Errorf("endpoint %s is on %s side, expected %s", endpointID, endpoint.Side, side)
	}
	if endpoint.BranchID != branchID {
		return EndpointAsset{}, fmt.Errorf("endpoint %s belongs to branch %s, not %s", endpointID, endpoint.BranchID, branchID)
	}
	return endpoint, nil
}

func validationFrom(kind AssetKind, id string, err error) ValidationIssue {
	return ValidationIssue{Kind: string(kind), AssetID: id, Message: err.Error()}
}

func zoneIn(c *Catalog, id string) (ZoneAsset, bool) {
	for _, zone := range c.Zones {
		if zone.ID == id {
			return zone, true
		}
	}
	return ZoneAsset{}, false
}

func findStation(c *Catalog, id string) (StationAsset, bool) {
	for _, station := range c.Stations {
		if station.ID == id {
			return station, true
		}
	}
	return StationAsset{}, false
}

func findBranch(c *Catalog, id string) (BranchAsset, bool) {
	for _, branch := range c.Branches {
		if branch.ID == id {
			return branch, true
		}
	}
	return BranchAsset{}, false
}

func findValve(c *Catalog, id string) (ValveAsset, bool) {
	for _, valve := range c.Valves {
		if valve.ID == id {
			return valve, true
		}
	}
	return ValveAsset{}, false
}

func findRelief(c *Catalog, id string) (ReliefAsset, bool) {
	for _, relief := range c.Reliefs {
		if relief.ID == id {
			return relief, true
		}
	}
	return ReliefAsset{}, false
}

func findEndpoint(c *Catalog, id string) (EndpointAsset, bool) {
	for _, endpoint := range c.Endpoints {
		if endpoint.ID == id {
			return endpoint, true
		}
	}
	return EndpointAsset{}, false
}

// StationZone 返回站点所属分区 ID。
func (c *Catalog) StationZone(stationID string) (string, bool) {
	station, ok := findStation(c, stationID)
	if !ok {
		return "", false
	}
	return station.ZoneID, true
}

// RequiredStationsForZone 返回分区内全部换热站 ID（这些站点是恢复投运的必要条件）。
func (c *Catalog) RequiredStationsForZone(zoneID string) []string {
	ids := make([]string, 0)
	for _, station := range c.Stations {
		if station.ZoneID == zoneID {
			ids = append(ids, station.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// StationBindings 从目录派生站点-分区绑定，供恢复逻辑复用。
func (c *Catalog) StationBindings() []StationBinding {
	bindings := make([]StationBinding, 0, len(c.Stations))
	for _, station := range c.Stations {
		bindings = append(bindings, StationBinding{StationID: station.ID, ZoneID: station.ZoneID, Required: true})
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].StationID < bindings[j].StationID })
	return bindings
}

// Sections 从目录派生拓扑分支定义，供隔离服务复用，
// 确保隔离操作与目录使用同一版本的供回水与安全隔离器件关系。
func (c *Catalog) Sections() []Section {
	sections := make([]Section, 0, len(c.Branches))
	for _, branch := range c.Branches {
		sections = append(sections, Section{
			ID: branch.ID, ZoneID: branch.ZoneID, Upstream: branch.SupplyEndpoint,
			Downstream: branch.ReturnEndpoint, BoundaryValves: append([]string(nil), branch.BoundaryValves...),
			ReliefValve: branch.ReliefValve,
		})
	}
	return sections
}

// Branch 返回一条分支的目录定义。
func (c *Catalog) Branch(branchID string) (BranchAsset, bool) {
	return findBranch(c, branchID)
}

// ExchangersForStation 返回站内全部换热器 ID。
func (c *Catalog) ExchangersForStation(stationID string) []string {
	ids := make([]string, 0)
	for _, exchanger := range c.Exchangers {
		if exchanger.StationID == stationID {
			ids = append(ids, exchanger.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// PumpsForZone 返回分区内全部循环泵 ID。
func (c *Catalog) PumpsForZone(zoneID string) []string {
	ids := make([]string, 0)
	for _, pump := range c.Pumps {
		if pump.ZoneID == zoneID {
			ids = append(ids, pump.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// ValvesForZone 返回分区内全部阀门 ID。
func (c *Catalog) ValvesForZone(zoneID string) []string {
	ids := make([]string, 0)
	for _, valve := range c.Valves {
		if valve.ZoneID == zoneID {
			ids = append(ids, valve.ID)
		}
	}
	sort.Strings(ids)
	return ids
}
