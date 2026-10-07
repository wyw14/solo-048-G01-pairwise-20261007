package assets

// Version 返回目录版本（schema + 内容修订摘要）。
func (c *Catalog) Version() Version {
	return Version{Schema: c.schemaVersion, Digest: c.revision}
}

// SchemaVersion 返回数据 schema 版本。
func (c *Catalog) SchemaVersion() int { return c.schemaVersion }

// Zones 按定义顺序返回全部分区。
func (c *Catalog) Zones() []Zone {
	result := make([]Zone, 0, len(c.zoneOrder))
	for _, id := range c.zoneOrder {
		result = append(result, c.zones[id])
	}
	return result
}

// Zone 返回单个分区。
func (c *Catalog) Zone(id string) (Zone, bool) {
	zone, ok := c.zones[id]
	return zone, ok
}

// Branches 按定义顺序返回全部分支。
func (c *Catalog) Branches() []Branch {
	result := make([]Branch, 0, len(c.branchOrder))
	for _, id := range c.branchOrder {
		result = append(result, c.branches[id])
	}
	return result
}

// Branch 返回单条分支。
func (c *Catalog) Branch(id string) (Branch, bool) {
	branch, ok := c.branches[id]
	return branch, ok
}

// BranchesForZone 返回某分区下的分支（按定义顺序）。
func (c *Catalog) BranchesForZone(zoneID string) []Branch {
	ids := c.branchesByZone[zoneID]
	result := make([]Branch, 0, len(ids))
	for _, id := range ids {
		if branch, ok := c.branches[id]; ok {
			result = append(result, branch)
		}
	}
	return result
}

// Node 返回供回水端点节点。
func (c *Catalog) Node(id string) (Node, bool) {
	node, ok := c.nodes[id]
	return node, ok
}

// Stations 按定义顺序返回全部换热站。
func (c *Catalog) Stations() []Station {
	result := make([]Station, 0, len(c.stationOrder))
	for _, id := range c.stationOrder {
		result = append(result, c.stations[id])
	}
	return result
}

// Station 返回单个换热站。
func (c *Catalog) Station(id string) (Station, bool) {
	station, ok := c.stations[id]
	return station, ok
}

// StationsForZone 返回某分区下的站点（按定义顺序）。
func (c *Catalog) StationsForZone(zoneID string) []Station {
	ids := c.stationsByZone[zoneID]
	result := make([]Station, 0, len(ids))
	for _, id := range ids {
		if station, ok := c.stations[id]; ok {
			result = append(result, station)
		}
	}
	return result
}

// RequiredStationsForZone 返回分区恢复时必须就位的站点 ID。
func (c *Catalog) RequiredStationsForZone(zoneID string) []string {
	var result []string
	for _, station := range c.StationsForZone(zoneID) {
		if station.Required {
			result = append(result, station.ID)
		}
	}
	return result
}

// Exchanger 返回换热器记录。
func (c *Catalog) Exchanger(id string) (Exchanger, bool) {
	ex, ok := c.exchangers[id]
	return ex, ok
}

// ExchangersForStation 返回站点的全部换热器（定义顺序）。
func (c *Catalog) ExchangersForStation(stationID string) []Exchanger {
	station, ok := c.stations[stationID]
	if !ok {
		return nil
	}
	result := make([]Exchanger, 0, len(station.Exchangers))
	for _, id := range station.Exchangers {
		if ex, ok := c.exchangers[id]; ok {
			result = append(result, ex)
		}
	}
	return result
}

// Pump 返回循环泵记录。
func (c *Catalog) Pump(id string) (Pump, bool) {
	p, ok := c.pumps[id]
	return p, ok
}

// PumpsForZone 返回分区下挂的循环泵（分区定义顺序）。
func (c *Catalog) PumpsForZone(zoneID string) []Pump {
	zone, ok := c.zones[zoneID]
	if !ok {
		return nil
	}
	result := make([]Pump, 0, len(zone.Pumps))
	for _, id := range zone.Pumps {
		if pump, ok := c.pumps[id]; ok {
			result = append(result, pump)
		}
	}
	return result
}

// Valve 返回普通阀门记录。
func (c *Catalog) Valve(id string) (Valve, bool) {
	v, ok := c.valves[id]
	return v, ok
}

// ValvesForStation 返回站点的全部普通阀门。
func (c *Catalog) ValvesForStation(stationID string) []Valve {
	station, ok := c.stations[stationID]
	if !ok {
		return nil
	}
	result := make([]Valve, 0, len(station.Valves))
	for _, id := range station.Valves {
		if v, ok := c.valves[id]; ok {
			result = append(result, v)
		}
	}
	return result
}

// Boundary 返回边界隔断阀记录。
func (c *Catalog) Boundary(id string) (BoundaryValve, bool) {
	bv, ok := c.boundaries[id]
	return bv, ok
}

// Relief 返回边界泄压设备记录。
func (c *Catalog) Relief(id string) (ReliefDevice, bool) {
	rd, ok := c.reliefs[id]
	return rd, ok
}

// Snapshot 是目录对外导出的完整只读视图，供 API 与页面统一引用。
type Snapshot struct {
	Version    Version         `json:"version"`
	Zones      []Zone          `json:"zones"`
	Branches   []Branch        `json:"branches"`
	Nodes      []Node          `json:"nodes"`
	Stations   []Station       `json:"stations"`
	Exchangers []Exchanger     `json:"exchangers"`
	Pumps      []Pump          `json:"pumps"`
	Valves     []Valve         `json:"valves"`
	Boundaries []BoundaryValve `json:"boundaries"`
	Reliefs    []ReliefDevice  `json:"reliefs"`
}

// Snapshot 导出目录的完整内容与版本，恢复、联锁、调度、运维页面消费同一载荷。
func (c *Catalog) Snapshot() Snapshot {
	return Snapshot{
		Version:    c.Version(),
		Zones:      c.Zones(),
		Branches:   c.Branches(),
		Nodes:      c.nodesInOrder(),
		Stations:   c.Stations(),
		Exchangers: c.exchangersInOrder(),
		Pumps:      c.pumpsInOrder(),
		Valves:     c.valvesInOrder(),
		Boundaries: c.boundariesInOrder(),
		Reliefs:    c.reliefsInOrder(),
	}
}

func (c *Catalog) nodesInOrder() []Node {
	result := make([]Node, 0, len(c.nodes))
	for _, id := range sortedKeys(c.nodes) {
		result = append(result, c.nodes[id])
	}
	return result
}

func (c *Catalog) exchangersInOrder() []Exchanger {
	result := make([]Exchanger, 0, len(c.exchangers))
	for _, id := range sortedKeys(c.exchangers) {
		result = append(result, c.exchangers[id])
	}
	return result
}

func (c *Catalog) pumpsInOrder() []Pump {
	result := make([]Pump, 0, len(c.pumps))
	for _, id := range sortedKeys(c.pumps) {
		result = append(result, c.pumps[id])
	}
	return result
}

func (c *Catalog) valvesInOrder() []Valve {
	result := make([]Valve, 0, len(c.valves))
	for _, id := range sortedKeys(c.valves) {
		result = append(result, c.valves[id])
	}
	return result
}

func (c *Catalog) boundariesInOrder() []BoundaryValve {
	result := make([]BoundaryValve, 0, len(c.boundaries))
	for _, id := range sortedKeys(c.boundaries) {
		result = append(result, c.boundaries[id])
	}
	return result
}

func (c *Catalog) reliefsInOrder() []ReliefDevice {
	result := make([]ReliefDevice, 0, len(c.reliefs))
	for _, id := range sortedKeys(c.reliefs) {
		result = append(result, c.reliefs[id])
	}
	return result
}
