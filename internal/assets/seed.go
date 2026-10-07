package assets

// DefaultDefinition 返回 HeatPulse 内置的供热网络资产定义。
// 这里是全网资产拓扑的唯一录入处：分区、分支、供回水端点、站点、换热器、
// 循环泵、普通阀门以及分支边界隔断阀/泄压设备都在此登记，
// 再由 Build 统一校验并生成带版本号的目录，运行期各子系统不得另存拓扑副本。
func DefaultDefinition() Definition {
	zones := []Zone{
		{ID: "north", Name: "北环", Branches: []string{"north-trunk"}, Pumps: []string{"north-cp"}},
		{ID: "east", Name: "东支线", Branches: []string{"east-branch"}, Pumps: []string{"east-cp"}},
		{ID: "west", Name: "西区", Branches: []string{"west-loop"}, Pumps: []string{"west-cp"}},
	}
	branches := []Branch{
		{
			ID: "north-trunk", ZoneID: "north", Name: "北环主干",
			SupplyNode: "north-supply", ReturnNode: "north-return",
			Stations:       []string{"xincheng"},
			SupplyBoundary: "north-bv-a", ReturnBoundary: "north-bv-b", Relief: "north-rv",
		},
		{
			ID: "east-branch", ZoneID: "east", Name: "东支线分支",
			SupplyNode: "east-supply", ReturnNode: "east-return",
			Stations:       []string{"binhe"},
			SupplyBoundary: "east-bv-a", ReturnBoundary: "east-bv-b", Relief: "east-rv",
		},
		{
			ID: "west-loop", ZoneID: "west", Name: "西区环路",
			SupplyNode: "west-supply", ReturnNode: "west-return",
			Stations:       []string{"nanhu", "xijiao"},
			SupplyBoundary: "west-bv-a", ReturnBoundary: "west-bv-b", Relief: "west-rv",
		},
	}
	nodes := []Node{
		{ID: "north-supply", BranchID: "north-trunk", Conduit: Supply, Name: "北环供水端"},
		{ID: "north-return", BranchID: "north-trunk", Conduit: Return, Name: "北环回水端"},
		{ID: "east-supply", BranchID: "east-branch", Conduit: Supply, Name: "东支线供水端"},
		{ID: "east-return", BranchID: "east-branch", Conduit: Return, Name: "东支线回水端"},
		{ID: "west-supply", BranchID: "west-loop", Conduit: Supply, Name: "西区供水端"},
		{ID: "west-return", BranchID: "west-loop", Conduit: Return, Name: "西区回水端"},
	}
	boundaries := []BoundaryValve{
		{ID: "north-bv-a", Name: "北环供水边界阀", BranchID: "north-trunk", Conduit: Supply},
		{ID: "north-bv-b", Name: "北环回水边界阀", BranchID: "north-trunk", Conduit: Return},
		{ID: "east-bv-a", Name: "东支线供水边界阀", BranchID: "east-branch", Conduit: Supply},
		{ID: "east-bv-b", Name: "东支线回水边界阀", BranchID: "east-branch", Conduit: Return},
		{ID: "west-bv-a", Name: "西区供水边界阀", BranchID: "west-loop", Conduit: Supply},
		{ID: "west-bv-b", Name: "西区回水边界阀", BranchID: "west-loop", Conduit: Return},
	}
	reliefs := []ReliefDevice{
		{ID: "north-rv", Name: "北环泄压阀", BranchID: "north-trunk"},
		{ID: "east-rv", Name: "东支线泄压阀", BranchID: "east-branch"},
		{ID: "west-rv", Name: "西区泄压阀", BranchID: "west-loop"},
	}
	pumps := []Pump{
		{ID: "north-cp", Name: "北环循环泵", ZoneID: "north", Stage: 2},
		{ID: "east-cp", Name: "东支线循环泵", ZoneID: "east", Stage: 2},
		{ID: "west-cp", Name: "西区循环泵", ZoneID: "west", Stage: 2},
	}

	type stationSeed struct {
		id, name, zone, branch string
		required               bool
	}
	stationSeeds := []stationSeed{
		{id: "xincheng", name: "新城站", zone: "north", branch: "north-trunk", required: true},
		{id: "binhe", name: "滨河站", zone: "east", branch: "east-branch", required: true},
		{id: "nanhu", name: "南湖站", zone: "west", branch: "west-loop", required: true},
		{id: "xijiao", name: "西郊站", zone: "west", branch: "west-loop", required: true},
	}
	stations := make([]Station, 0, len(stationSeeds))
	exchangers := make([]Exchanger, 0, len(stationSeeds)*2)
	valves := make([]Valve, 0, len(stationSeeds)*3)
	for _, seed := range stationSeeds {
		stations = append(stations, Station{
			ID: seed.id, Name: seed.name, ZoneID: seed.zone, BranchID: seed.branch, Required: seed.required,
			Exchangers: []string{seed.id + "-a", seed.id + "-b"},
			Valves:     []string{seed.id + "-primary", seed.id + "-return", seed.id + "-manual"},
		})
		exchangers = append(exchangers,
			Exchanger{ID: seed.id + "-a", StationID: seed.id, Train: "A", Name: seed.name + " A 列换热器"},
			Exchanger{ID: seed.id + "-b", StationID: seed.id, Train: "B", Name: seed.name + " B 列换热器"},
		)
		valves = append(valves,
			Valve{ID: seed.id + "-primary", Name: seed.name + "一次网阀", StationID: seed.id, Role: RolePrimary},
			Valve{ID: seed.id + "-return", Name: seed.name + "回水防冻阀", StationID: seed.id, Role: RoleFrost},
			Valve{ID: seed.id + "-manual", Name: seed.name + "手动调节阀", StationID: seed.id, Role: RoleManual},
		)
	}

	return Definition{
		SchemaVersion: 1,
		Zones:         zones,
		Branches:      branches,
		Nodes:         nodes,
		Stations:      stations,
		Exchangers:    exchangers,
		Pumps:         pumps,
		Valves:        valves,
		Boundaries:    boundaries,
		Reliefs:       reliefs,
	}
}

// DefaultCatalog 构建内置资产目录，供应用启动使用；校验失败直接 panic，
// 因为内置目录无效属于不可恢复的程序缺陷（测试与启动路径共用同一入口）。
func DefaultCatalog() *Catalog {
	catalog, err := Build(DefaultDefinition())
	if err != nil {
		panic(err)
	}
	return catalog
}
