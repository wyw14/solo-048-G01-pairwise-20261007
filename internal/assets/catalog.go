package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Catalog 是不可变的、带版本号的供热网络资产目录。
// 所有子系统（联锁、恢复、调度、运维、页面）都持有同一 *Catalog 指针，
// 通过 Version() 读取同一版本，杜绝各处各自维护拓扑副本。
type Catalog struct {
	schemaVersion int
	revision      string

	zones      map[string]Zone
	branches   map[string]Branch
	nodes      map[string]Node
	stations   map[string]Station
	exchangers map[string]Exchanger
	pumps      map[string]Pump
	valves     map[string]Valve
	boundaries map[string]BoundaryValve
	reliefs    map[string]ReliefDevice

	zoneOrder    []string
	branchOrder  []string
	stationOrder []string

	stationsByZone   map[string][]string
	stationsByBranch map[string][]string
	branchesByZone   map[string][]string
}

// Version 唯一标识当前目录内容。schema 版本来自定义数据，
// revision 是全部资产拓扑内容的确定性哈希，任一资产或挂接关系变化都会产生新版本。
type Version struct {
	Schema int    `json:"schema"`
	Digest string `json:"digest"`
}

func (v Version) String() string {
	return fmt.Sprintf("catalog-v%d-%s", v.Schema, v.Digest)
}

// Build 构建目录并执行启动校验。校验失败时返回错误，服务不得启动。
func Build(def Definition) (*Catalog, error) {
	if def.SchemaVersion < 1 {
		return nil, fmt.Errorf("asset catalog: schema_version must be >= 1")
	}
	c := &Catalog{
		schemaVersion:    def.SchemaVersion,
		zones:            make(map[string]Zone, len(def.Zones)),
		branches:         make(map[string]Branch, len(def.Branches)),
		nodes:            make(map[string]Node, len(def.Nodes)),
		stations:         make(map[string]Station, len(def.Stations)),
		exchangers:       make(map[string]Exchanger, len(def.Exchangers)),
		pumps:            make(map[string]Pump, len(def.Pumps)),
		valves:           make(map[string]Valve, len(def.Valves)),
		boundaries:       make(map[string]BoundaryValve, len(def.Boundaries)),
		reliefs:          make(map[string]ReliefDevice, len(def.Reliefs)),
		stationsByZone:   make(map[string][]string),
		stationsByBranch: make(map[string][]string),
		branchesByZone:   make(map[string][]string),
	}

	// 第一遍：登记全部资产并校验全局唯一 ID。
	seen := make(map[string]Kind)
	register := func(id string, kind Kind) error {
		if id == "" {
			return fmt.Errorf("asset catalog: %s has empty id", kind)
		}
		if previous, ok := seen[id]; ok {
			return fmt.Errorf("asset catalog: duplicate id %q used by %s and %s", id, previous, kind)
		}
		seen[id] = kind
		return nil
	}
	for _, z := range def.Zones {
		if err := register(z.ID, KindZone); err != nil {
			return nil, err
		}
		c.zones[z.ID] = cloneZone(z)
		c.zoneOrder = append(c.zoneOrder, z.ID)
	}
	for _, b := range def.Branches {
		if err := register(b.ID, "branch"); err != nil {
			return nil, err
		}
		c.branches[b.ID] = cloneBranch(b)
		c.branchOrder = append(c.branchOrder, b.ID)
	}
	for _, n := range def.Nodes {
		if err := register(n.ID, "node"); err != nil {
			return nil, err
		}
		if n.Conduit != Supply && n.Conduit != Return {
			return nil, fmt.Errorf("asset catalog: node %s has invalid conduit %q", n.ID, n.Conduit)
		}
		c.nodes[n.ID] = n
	}
	for _, s := range def.Stations {
		if err := register(s.ID, KindStation); err != nil {
			return nil, err
		}
		c.stations[s.ID] = cloneStation(s)
		c.stationOrder = append(c.stationOrder, s.ID)
	}
	for _, e := range def.Exchangers {
		if err := register(e.ID, KindExchanger); err != nil {
			return nil, err
		}
		if e.Train == "" {
			return nil, fmt.Errorf("asset catalog: exchanger %s missing train", e.ID)
		}
		c.exchangers[e.ID] = e
	}
	for _, p := range def.Pumps {
		if err := register(p.ID, KindPump); err != nil {
			return nil, err
		}
		c.pumps[p.ID] = p
	}
	for _, v := range def.Valves {
		if err := register(v.ID, KindValve); err != nil {
			return nil, err
		}
		if v.Role == "" {
			return nil, fmt.Errorf("asset catalog: valve %s missing role", v.ID)
		}
		c.valves[v.ID] = v
	}
	for _, bv := range def.Boundaries {
		if err := register(bv.ID, KindBoundary); err != nil {
			return nil, err
		}
		if bv.Conduit != Supply && bv.Conduit != Return {
			return nil, fmt.Errorf("asset catalog: boundary valve %s has invalid conduit %q", bv.ID, bv.Conduit)
		}
		c.boundaries[bv.ID] = bv
	}
	for _, rd := range def.Reliefs {
		if err := register(rd.ID, KindRelief); err != nil {
			return nil, err
		}
		c.reliefs[rd.ID] = rd
	}

	if err := c.validateReferences(); err != nil {
		return nil, err
	}
	if err := c.validateCoverage(); err != nil {
		return nil, err
	}

	c.revision = c.computeDigest()
	return c, nil
}

// validateReferences 校验所有跨表引用都能解析，并保证供回水端点、隔离器件的挂接一致。
func (c *Catalog) validateReferences() error {
	for _, zone := range c.zones {
		for _, branchID := range zone.Branches {
			branch, ok := c.branches[branchID]
			if !ok {
				return fmt.Errorf("asset catalog: zone %s references unknown branch %s", zone.ID, branchID)
			}
			if branch.ZoneID != zone.ID {
				return fmt.Errorf("asset catalog: branch %s is attached to zone %s but listed under %s", branchID, branch.ZoneID, zone.ID)
			}
			c.branchesByZone[zone.ID] = append(c.branchesByZone[zone.ID], branchID)
		}
		for _, pumpID := range zone.Pumps {
			pump, ok := c.pumps[pumpID]
			if !ok {
				return fmt.Errorf("asset catalog: zone %s references unknown pump %s", zone.ID, pumpID)
			}
			if pump.ZoneID != zone.ID {
				return fmt.Errorf("asset catalog: pump %s attached to zone %s but listed under %s", pumpID, pump.ZoneID, zone.ID)
			}
		}
	}

	for _, branch := range c.branches {
		if _, ok := c.zones[branch.ZoneID]; !ok {
			return fmt.Errorf("asset catalog: branch %s references unknown zone %s", branch.ID, branch.ZoneID)
		}
		if _, ok := c.zones[branch.SupplyNode]; ok {
			return fmt.Errorf("asset catalog: branch %s supply_node %s is a zone id, expected a node", branch.ID, branch.SupplyNode)
		}
		supply, ok := c.nodes[branch.SupplyNode]
		if !ok {
			return fmt.Errorf("asset catalog: branch %s references unknown supply node %s", branch.ID, branch.SupplyNode)
		}
		ret, ok := c.nodes[branch.ReturnNode]
		if !ok {
			return fmt.Errorf("asset catalog: branch %s references unknown return node %s", branch.ID, branch.ReturnNode)
		}
		// 启动时校验每条分支的供回水端点：必须一供一回，且都属于本分支。
		if supply.Conduit != Supply {
			return fmt.Errorf("asset catalog: branch %s supply endpoint %s is on %s conduit", branch.ID, supply.ID, supply.Conduit)
		}
		if ret.Conduit != Return {
			return fmt.Errorf("asset catalog: branch %s return endpoint %s is on %s conduit", branch.ID, ret.ID, ret.Conduit)
		}
		if supply.BranchID != branch.ID || ret.BranchID != branch.ID {
			return fmt.Errorf("asset catalog: branch %s endpoints %s/%s are not both attached to it", branch.ID, supply.ID, ret.ID)
		}
		// 启动时校验安全隔离器件：供/回水边界阀 + 泄压设备齐全、类型与挂接正确。
		if err := c.validateIsolation(branch); err != nil {
			return err
		}
		for _, stationID := range branch.Stations {
			station, ok := c.stations[stationID]
			if !ok {
				return fmt.Errorf("asset catalog: branch %s references unknown station %s", branch.ID, stationID)
			}
			if station.BranchID != branch.ID {
				return fmt.Errorf("asset catalog: station %s attached to branch %s but listed under %s", stationID, station.BranchID, branch.ID)
			}
			if station.ZoneID != branch.ZoneID {
				return fmt.Errorf("asset catalog: station %s zone %s disagrees with branch %s zone %s", stationID, station.ZoneID, branch.ID, branch.ZoneID)
			}
			c.stationsByBranch[branch.ID] = append(c.stationsByBranch[branch.ID], stationID)
			c.stationsByZone[branch.ZoneID] = append(c.stationsByZone[branch.ZoneID], stationID)
		}
	}

	for _, station := range c.stations {
		if _, ok := c.zones[station.ZoneID]; !ok {
			return fmt.Errorf("asset catalog: station %s references unknown zone %s", station.ID, station.ZoneID)
		}
		if _, ok := c.branches[station.BranchID]; !ok {
			return fmt.Errorf("asset catalog: station %s references unknown branch %s", station.ID, station.BranchID)
		}
		for _, exID := range station.Exchangers {
			ex, ok := c.exchangers[exID]
			if !ok {
				return fmt.Errorf("asset catalog: station %s references unknown exchanger %s", station.ID, exID)
			}
			if ex.StationID != station.ID {
				return fmt.Errorf("asset catalog: exchanger %s attached to station %s but listed under %s", exID, ex.StationID, station.ID)
			}
		}
		for _, valveID := range station.Valves {
			valve, ok := c.valves[valveID]
			if !ok {
				return fmt.Errorf("asset catalog: station %s references unknown valve %s", station.ID, valveID)
			}
			if valve.StationID != station.ID {
				return fmt.Errorf("asset catalog: valve %s attached to station %s but listed under %s", valveID, valve.StationID, station.ID)
			}
		}
	}

	for _, pump := range c.pumps {
		if _, ok := c.zones[pump.ZoneID]; !ok {
			return fmt.Errorf("asset catalog: pump %s references unknown zone %s", pump.ID, pump.ZoneID)
		}
		if pump.Station != "" {
			if _, ok := c.stations[pump.Station]; !ok {
				return fmt.Errorf("asset catalog: pump %s references unknown station %s", pump.ID, pump.Station)
			}
		}
	}
	// 换热器、普通阀门的反向引用已在站点遍历时覆盖，这里仅保证其所属实体存在。
	for _, ex := range c.exchangers {
		if _, ok := c.stations[ex.StationID]; !ok {
			return fmt.Errorf("asset catalog: exchanger %s references unknown station %s", ex.ID, ex.StationID)
		}
	}
	for _, v := range c.valves {
		if _, ok := c.stations[v.StationID]; !ok {
			return fmt.Errorf("asset catalog: valve %s references unknown station %s", v.ID, v.StationID)
		}
	}
	for _, bv := range c.boundaries {
		if _, ok := c.branches[bv.BranchID]; !ok {
			return fmt.Errorf("asset catalog: boundary valve %s references unknown branch %s", bv.ID, bv.BranchID)
		}
	}
	for _, rd := range c.reliefs {
		if _, ok := c.branches[rd.BranchID]; !ok {
			return fmt.Errorf("asset catalog: relief device %s references unknown branch %s", rd.ID, rd.BranchID)
		}
	}
	return nil
}

// validateIsolation 校验单条分支的安全隔离器件配置。
func (c *Catalog) validateIsolation(branch Branch) error {
	if branch.SupplyBoundary == "" || branch.ReturnBoundary == "" {
		return fmt.Errorf("asset catalog: branch %s lacks supply/return boundary valves", branch.ID)
	}
	if branch.SupplyBoundary == branch.ReturnBoundary {
		return fmt.Errorf("asset catalog: branch %s reuses one boundary valve on both conduits", branch.ID)
	}
	if branch.Relief == "" {
		return fmt.Errorf("asset catalog: branch %s lacks relief device", branch.ID)
	}
	supplyBV, ok := c.boundaries[branch.SupplyBoundary]
	if !ok {
		return fmt.Errorf("asset catalog: branch %s references unknown supply boundary %s", branch.ID, branch.SupplyBoundary)
	}
	returnBV, ok := c.boundaries[branch.ReturnBoundary]
	if !ok {
		return fmt.Errorf("asset catalog: branch %s references unknown return boundary %s", branch.ID, branch.ReturnBoundary)
	}
	relief, ok := c.reliefs[branch.Relief]
	if !ok {
		return fmt.Errorf("asset catalog: branch %s references unknown relief device %s", branch.ID, branch.Relief)
	}
	if supplyBV.Conduit != Supply {
		return fmt.Errorf("asset catalog: branch %s supply boundary %s is on %s conduit", branch.ID, supplyBV.ID, supplyBV.Conduit)
	}
	if returnBV.Conduit != Return {
		return fmt.Errorf("asset catalog: branch %s return boundary %s is on %s conduit", branch.ID, returnBV.ID, returnBV.Conduit)
	}
	if supplyBV.BranchID != branch.ID || returnBV.BranchID != branch.ID {
		return fmt.Errorf("asset catalog: branch %s boundary valves are not both attached to it", branch.ID)
	}
	if relief.BranchID != branch.ID {
		return fmt.Errorf("asset catalog: branch %s relief device %s is attached to branch %s", branch.ID, relief.ID, relief.BranchID)
	}
	return nil
}

// validateCoverage 保证每个分区至少有一条可隔离分支，且每条分支都有泄压路径，
// 同时避免资产“悬空”（登记了却没有挂到任何父实体）。
func (c *Catalog) validateCoverage() error {
	for _, zone := range c.zones {
		if len(zone.Branches) == 0 {
			return fmt.Errorf("asset catalog: zone %s has no branch", zone.ID)
		}
	}
	usedNode := make(map[string]bool)
	usedBoundary := make(map[string]bool)
	usedRelief := make(map[string]bool)
	usedPump := make(map[string]bool)
	usedExchanger := make(map[string]bool)
	usedValve := make(map[string]bool)
	for _, zone := range c.zones {
		for _, pumpID := range zone.Pumps {
			usedPump[pumpID] = true
		}
	}
	for _, branch := range c.branches {
		usedNode[branch.SupplyNode] = true
		usedNode[branch.ReturnNode] = true
		usedBoundary[branch.SupplyBoundary] = true
		usedBoundary[branch.ReturnBoundary] = true
		usedRelief[branch.Relief] = true
	}
	for _, station := range c.stations {
		for _, id := range station.Exchangers {
			usedExchanger[id] = true
		}
		for _, id := range station.Valves {
			usedValve[id] = true
		}
	}
	for id := range c.nodes {
		if !usedNode[id] {
			return fmt.Errorf("asset catalog: node %s is not used as a branch endpoint", id)
		}
	}
	for id := range c.boundaries {
		if !usedBoundary[id] {
			return fmt.Errorf("asset catalog: boundary valve %s is not assigned to a branch", id)
		}
	}
	for id := range c.reliefs {
		if !usedRelief[id] {
			return fmt.Errorf("asset catalog: relief device %s is not assigned to a branch", id)
		}
	}
	for id := range c.pumps {
		if !usedPump[id] {
			return fmt.Errorf("asset catalog: pump %s is not assigned to a zone", id)
		}
	}
	for id := range c.exchangers {
		if !usedExchanger[id] {
			return fmt.Errorf("asset catalog: exchanger %s is not assigned to a station", id)
		}
	}
	for id := range c.valves {
		if !usedValve[id] {
			return fmt.Errorf("asset catalog: valve %s is not assigned to a station", id)
		}
	}
	// 每个站点至少一台换热器（否则恢复暖管无对象）。
	for _, station := range c.stations {
		if len(station.Exchangers) == 0 {
			return fmt.Errorf("asset catalog: station %s has no exchanger", station.ID)
		}
	}
	return nil
}

// computeDigest 对目录全部拓扑内容做确定性哈希，作为内容修订号。
func (c *Catalog) computeDigest() string {
	var b strings.Builder
	fmt.Fprintf(&b, "schema=%d\n", c.schemaVersion)

	writeZone := func(z Zone) {
		fmt.Fprintf(&b, "zone %s name=%q branches=%v pumps=%v\n", z.ID, z.Name, z.Branches, z.Pumps)
	}
	writeBranch := func(br Branch) {
		fmt.Fprintf(&b, "branch %s zone=%s name=%q supply=%s return=%s stations=%v sb=%s rb=%s relief=%s\n",
			br.ID, br.ZoneID, br.Name, br.SupplyNode, br.ReturnNode, br.Stations, br.SupplyBoundary, br.ReturnBoundary, br.Relief)
	}
	for _, id := range c.zoneOrder {
		writeZone(c.zones[id])
	}
	for _, id := range c.branchOrder {
		writeBranch(c.branches[id])
	}
	nodeIDs := sortedKeys(c.nodes)
	for _, id := range nodeIDs {
		n := c.nodes[id]
		fmt.Fprintf(&b, "node %s branch=%s conduit=%s name=%q\n", n.ID, n.BranchID, n.Conduit, n.Name)
	}
	for _, id := range c.stationOrder {
		s := c.stations[id]
		fmt.Fprintf(&b, "station %s name=%q zone=%s branch=%s required=%t exchangers=%v valves=%v\n",
			s.ID, s.Name, s.ZoneID, s.BranchID, s.Required, s.Exchangers, s.Valves)
	}
	for _, id := range sortedKeys(c.exchangers) {
		e := c.exchangers[id]
		fmt.Fprintf(&b, "exchanger %s station=%s train=%s name=%q\n", e.ID, e.StationID, e.Train, e.Name)
	}
	for _, id := range sortedKeys(c.pumps) {
		p := c.pumps[id]
		fmt.Fprintf(&b, "pump %s zone=%s station=%s stage=%d name=%q\n", p.ID, p.ZoneID, p.Station, p.Stage, p.Name)
	}
	for _, id := range sortedKeys(c.valves) {
		v := c.valves[id]
		fmt.Fprintf(&b, "valve %s station=%s role=%s name=%q\n", v.ID, v.StationID, v.Role, v.Name)
	}
	for _, id := range sortedKeys(c.boundaries) {
		bv := c.boundaries[id]
		fmt.Fprintf(&b, "boundary %s branch=%s conduit=%s name=%q\n", bv.ID, bv.BranchID, bv.Conduit, bv.Name)
	}
	for _, id := range sortedKeys(c.reliefs) {
		rd := c.reliefs[id]
		fmt.Fprintf(&b, "relief %s branch=%s name=%q\n", rd.ID, rd.BranchID, rd.Name)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])[:12]
}

func sortedKeys[V any](m map[string]V) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func cloneZone(z Zone) Zone {
	z.Branches = append([]string(nil), z.Branches...)
	z.Pumps = append([]string(nil), z.Pumps...)
	return z
}

func cloneBranch(b Branch) Branch {
	b.Stations = append([]string(nil), b.Stations...)
	return b
}

func cloneStation(s Station) Station {
	s.Exchangers = append([]string(nil), s.Exchangers...)
	s.Valves = append([]string(nil), s.Valves...)
	return s
}
