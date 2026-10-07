package api

import "heatpulse/internal/network"

// seedCatalog 构建启动时加载的供热网络资产目录。
// 分区、站点、换热器、泵、阀门与边界泄压设备之间的拓扑关系在此唯一声明，
// 其余模块（状态、拓扑、绑定、恢复、联锁、调度、运维）一律从该目录派生。
func seedCatalog() *network.Catalog {
	zones := []network.ZoneAsset{
		{ID: "north", Name: "北环"},
		{ID: "east", Name: "东支线"},
		{ID: "west", Name: "西区"},
	}
	stations := []network.StationAsset{
		{ID: "xincheng", Name: "新城站", ZoneID: "north"},
		{ID: "binhe", Name: "滨河站", ZoneID: "east"},
		{ID: "nanhu", Name: "南湖站", ZoneID: "west"},
		{ID: "xijiao", Name: "西郊站", ZoneID: "west"},
	}
	exchangers := []network.ExchangerAsset{
		{ID: "xincheng-a", StationID: "xincheng"}, {ID: "xincheng-b", StationID: "xincheng"},
		{ID: "binhe-a", StationID: "binhe"}, {ID: "binhe-b", StationID: "binhe"},
		{ID: "nanhu-a", StationID: "nanhu"}, {ID: "nanhu-b", StationID: "nanhu"},
		{ID: "xijiao-a", StationID: "xijiao"}, {ID: "xijiao-b", StationID: "xijiao"},
	}
	pumps := []network.PumpAsset{
		{ID: "north-main-pump", ZoneID: "north"},
		{ID: "east-main-pump", ZoneID: "east"},
		{ID: "west-main-pump", ZoneID: "west"},
		{ID: "xincheng-pump", ZoneID: "north", StationID: "xincheng"},
		{ID: "binhe-pump", ZoneID: "east", StationID: "binhe"},
		{ID: "nanhu-pump", ZoneID: "west", StationID: "nanhu"},
		{ID: "xijiao-pump", ZoneID: "west", StationID: "xijiao"},
	}
	valves := []network.ValveAsset{
		{ID: "north-bv-a", Role: network.ValveBoundary, ZoneID: "north"},
		{ID: "north-bv-b", Role: network.ValveBoundary, ZoneID: "north"},
		{ID: "east-bv-a", Role: network.ValveBoundary, ZoneID: "east"},
		{ID: "east-bv-b", Role: network.ValveBoundary, ZoneID: "east"},
		{ID: "west-bv-a", Role: network.ValveBoundary, ZoneID: "west"},
		{ID: "west-bv-b", Role: network.ValveBoundary, ZoneID: "west"},
		{ID: "xincheng-bv-a", Role: network.ValveBoundary, ZoneID: "north", StationID: "xincheng"},
		{ID: "xincheng-bv-b", Role: network.ValveBoundary, ZoneID: "north", StationID: "xincheng"},
		{ID: "binhe-bv-a", Role: network.ValveBoundary, ZoneID: "east", StationID: "binhe"},
		{ID: "binhe-bv-b", Role: network.ValveBoundary, ZoneID: "east", StationID: "binhe"},
		{ID: "nanhu-bv-a", Role: network.ValveBoundary, ZoneID: "west", StationID: "nanhu"},
		{ID: "nanhu-bv-b", Role: network.ValveBoundary, ZoneID: "west", StationID: "nanhu"},
		{ID: "xijiao-bv-a", Role: network.ValveBoundary, ZoneID: "west", StationID: "xijiao"},
		{ID: "xijiao-bv-b", Role: network.ValveBoundary, ZoneID: "west", StationID: "xijiao"},
		{ID: "north-bypass", Role: network.ValveBypass, ZoneID: "north"},
		{ID: "east-bypass", Role: network.ValveBypass, ZoneID: "east"},
		{ID: "west-bypass", Role: network.ValveBypass, ZoneID: "west"},
		{ID: "xincheng-mv", Role: network.ValveManual, ZoneID: "north", StationID: "xincheng"},
		{ID: "binhe-mv", Role: network.ValveManual, ZoneID: "east", StationID: "binhe"},
		{ID: "nanhu-mv", Role: network.ValveManual, ZoneID: "west", StationID: "nanhu"},
		{ID: "xijiao-mv", Role: network.ValveManual, ZoneID: "west", StationID: "xijiao"},
	}
	reliefs := []network.ReliefAsset{
		{ID: "north-rv", ZoneID: "north", BranchID: "north-trunk"},
		{ID: "east-rv", ZoneID: "east", BranchID: "east-branch"},
		{ID: "west-rv", ZoneID: "west", BranchID: "west-loop"},
		{ID: "xincheng-rv", ZoneID: "north", BranchID: "xincheng-branch"},
		{ID: "binhe-rv", ZoneID: "east", BranchID: "binhe-branch"},
		{ID: "nanhu-rv", ZoneID: "west", BranchID: "nanhu-branch"},
		{ID: "xijiao-rv", ZoneID: "west", BranchID: "xijiao-branch"},
	}
	endpoints := []network.EndpointAsset{
		{ID: "north-supply", BranchID: "north-trunk", Side: network.SideSupply},
		{ID: "north-return", BranchID: "north-trunk", Side: network.SideReturn},
		{ID: "east-supply", BranchID: "east-branch", Side: network.SideSupply},
		{ID: "east-return", BranchID: "east-branch", Side: network.SideReturn},
		{ID: "west-supply", BranchID: "west-loop", Side: network.SideSupply},
		{ID: "west-return", BranchID: "west-loop", Side: network.SideReturn},
		{ID: "xincheng-supply", BranchID: "xincheng-branch", Side: network.SideSupply},
		{ID: "xincheng-return", BranchID: "xincheng-branch", Side: network.SideReturn},
		{ID: "binhe-supply", BranchID: "binhe-branch", Side: network.SideSupply},
		{ID: "binhe-return", BranchID: "binhe-branch", Side: network.SideReturn},
		{ID: "nanhu-supply", BranchID: "nanhu-branch", Side: network.SideSupply},
		{ID: "nanhu-return", BranchID: "nanhu-branch", Side: network.SideReturn},
		{ID: "xijiao-supply", BranchID: "xijiao-branch", Side: network.SideSupply},
		{ID: "xijiao-return", BranchID: "xijiao-branch", Side: network.SideReturn},
	}
	branches := []network.BranchAsset{
		{
			ID: "north-trunk", ZoneID: "north",
			SupplyEndpoint: "north-supply", ReturnEndpoint: "north-return",
			BoundaryValves: []string{"north-bv-a", "north-bv-b"}, ReliefValve: "north-rv",
		},
		{
			ID: "east-branch", ZoneID: "east",
			SupplyEndpoint: "east-supply", ReturnEndpoint: "east-return",
			BoundaryValves: []string{"east-bv-a", "east-bv-b"}, ReliefValve: "east-rv",
		},
		{
			ID: "west-loop", ZoneID: "west",
			SupplyEndpoint: "west-supply", ReturnEndpoint: "west-return",
			BoundaryValves: []string{"west-bv-a", "west-bv-b"}, ReliefValve: "west-rv",
		},
		{
			ID: "xincheng-branch", ZoneID: "north", StationID: "xincheng",
			SupplyEndpoint: "xincheng-supply", ReturnEndpoint: "xincheng-return",
			BoundaryValves: []string{"xincheng-bv-a", "xincheng-bv-b"}, ReliefValve: "xincheng-rv",
		},
		{
			ID: "binhe-branch", ZoneID: "east", StationID: "binhe",
			SupplyEndpoint: "binhe-supply", ReturnEndpoint: "binhe-return",
			BoundaryValves: []string{"binhe-bv-a", "binhe-bv-b"}, ReliefValve: "binhe-rv",
		},
		{
			ID: "nanhu-branch", ZoneID: "west", StationID: "nanhu",
			SupplyEndpoint: "nanhu-supply", ReturnEndpoint: "nanhu-return",
			BoundaryValves: []string{"nanhu-bv-a", "nanhu-bv-b"}, ReliefValve: "nanhu-rv",
		},
		{
			ID: "xijiao-branch", ZoneID: "west", StationID: "xijiao",
			SupplyEndpoint: "xijiao-supply", ReturnEndpoint: "xijiao-return",
			BoundaryValves: []string{"xijiao-bv-a", "xijiao-bv-b"}, ReliefValve: "xijiao-rv",
		},
	}
	return network.NewCatalog(zones, stations, exchangers, pumps, valves, reliefs, endpoints, branches)
}

// stationTargets 保存各站点的初始目标温度（℃）。
func stationTargets() map[string]float64 {
	return map[string]float64{"xincheng": 68, "binhe": 66, "nanhu": 64, "xijiao": 65}
}
