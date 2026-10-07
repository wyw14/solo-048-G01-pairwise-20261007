package api

import (
	"heatpulse/internal/assets"
	"heatpulse/internal/network"
	"heatpulse/internal/station"
)

// 分区初始运行参数：键为分区 ID。目录负责“有什么、怎么连”，这里仅给启动运行态赋值。
type initialZoneParams struct {
	phase                          network.Phase
	stage                          int
	bypass                         bool
	supplyPressure, returnPressure float64
}

var initialZoneConfig = map[string]initialZoneParams{
	"north": {network.PhaseActive, 2, true, 0.82, 0.48},
	"east":  {network.PhaseActive, 2, true, 0.76, 0.45},
	"west":  {network.PhaseActive, 2, true, 0.79, 0.46},
}

// zonesFromCatalog 从资产目录派生分区运行态的初始记录。
func zonesFromCatalog(catalog *assets.Catalog) []network.Zone {
	zones := make([]network.Zone, 0)
	for _, z := range catalog.Zones() {
		params, ok := initialZoneConfig[z.ID]
		if !ok {
			params = initialZoneParams{phase: network.PhaseOffline}
		}
		zones = append(zones, network.Zone{
			ID: z.ID, Name: z.Name, Phase: params.phase, PumpStage: params.stage,
			BypassOpen: params.bypass, SupplyPressure: params.supplyPressure, ReturnPressure: params.returnPressure,
		})
	}
	return zones
}

// topologyFromCatalog 把目录分支适配为隔离服务使用的 Section 拓扑，
// 供回水管端与安全隔离器件全部取自目录，不再手工另录。
func topologyFromCatalog(catalog *assets.Catalog) []network.Section {
	sections := make([]network.Section, 0)
	for _, branch := range catalog.Branches() {
		sections = append(sections, network.Section{
			ID: branch.ID, ZoneID: branch.ZoneID,
			Upstream: branch.SupplyNode, Downstream: branch.ReturnNode,
			BoundaryValves: []string{branch.SupplyBoundary, branch.ReturnBoundary},
			ReliefValve:    branch.Relief,
		})
	}
	return sections
}

// stationsFromCatalog 从目录派生站点注册表初始记录：A 列换热器带载、B 列备用。
func stationsFromCatalog(catalog *assets.Catalog) []station.Station {
	items := make([]station.Station, 0)
	for _, s := range catalog.Stations() {
		exchangers := make(map[string]station.ExchangerPhase, len(s.Exchangers))
		for _, ex := range catalog.ExchangersForStation(s.ID) {
			phase := station.ExchangerStandby
			if ex.Train == "A" {
				phase = station.ExchangerCarrying
			}
			exchangers[ex.Train] = phase
		}
		items = append(items, station.Station{
			ID: s.ID, Name: s.Name, ZoneID: s.ZoneID, Online: true,
			TargetCelsius: defaultTargetCelsius(s.ZoneID), TargetRevision: 1,
			ControllerEpoch: 1, Exchangers: exchangers,
		})
	}
	return items
}

func defaultTargetCelsius(zoneID string) float64 {
	switch zoneID {
	case "north":
		return 68
	case "east":
		return 66
	case "west":
		return 64
	default:
		return 65
	}
}
