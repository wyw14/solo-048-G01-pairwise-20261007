package recovery

import (
	"fmt"
	"sync"

	"heatpulse/internal/assets"
	"heatpulse/internal/exchanger"
	"heatpulse/internal/network"
	"heatpulse/internal/pump"
)

type ZoneRecovery struct {
	state     *network.State
	catalog   *assets.Catalog
	warmup    *exchanger.WarmupService
	pressure  *pump.PressureMonitor
	mu        sync.Mutex
	connected map[string]map[string]bool
}

// NewZoneRecovery 基于唯一资产目录构建分区恢复：必须就位的站点清单与分区身份都来自目录，
// 恢复判定输出统一加盖目录版本，保证恢复页面与调度/运维看到同一拓扑版本。
func NewZoneRecovery(state *network.State, catalog *assets.Catalog, warmup *exchanger.WarmupService, pressure *pump.PressureMonitor) *ZoneRecovery {
	return &ZoneRecovery{state: state, catalog: catalog, warmup: warmup, pressure: pressure, connected: make(map[string]map[string]bool)}
}

// CatalogVersion 返回恢复流程引用的目录版本。
func (r *ZoneRecovery) CatalogVersion() assets.Version { return r.catalog.Version() }

func (r *ZoneRecovery) Begin(zoneID string) (network.Zone, error) {
	if _, ok := r.catalog.Zone(zoneID); !ok {
		return network.Zone{}, fmt.Errorf("zone %s not found in asset catalog", zoneID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connected[zoneID] = make(map[string]bool)
	return r.state.Update(zoneID, func(zone *network.Zone) error {
		zone.RecoveryRevision++
		zone.Phase = network.PhaseWarming
		return nil
	})
}

func (r *ZoneRecovery) OnConnected(zoneID, stationID string) (network.Zone, error) {
	if _, ok := r.catalog.Zone(zoneID); !ok {
		return network.Zone{}, fmt.Errorf("zone %s not found in asset catalog", zoneID)
	}
	station, ok := r.catalog.Station(stationID)
	if !ok || station.ZoneID != zoneID {
		return network.Zone{}, fmt.Errorf("station %s is not attached to zone %s in asset catalog", stationID, zoneID)
	}
	r.mu.Lock()
	if r.connected[zoneID] == nil {
		r.connected[zoneID] = make(map[string]bool)
	}
	r.connected[zoneID][stationID] = true
	r.mu.Unlock()
	return r.evaluate(zoneID)
}

func (r *ZoneRecovery) Evaluate(zoneID string) (network.Zone, error) {
	return r.evaluate(zoneID)
}

func (r *ZoneRecovery) evaluate(zoneID string) (network.Zone, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	zone, ok := r.state.Get(zoneID)
	if !ok {
		return network.Zone{}, fmt.Errorf("zone %s not found", zoneID)
	}
	for _, stationID := range r.catalog.RequiredStationsForZone(zoneID) {
		if !r.connected[zoneID][stationID] || !r.warmup.Ready(stationID, zone.RecoveryRevision) || !r.pressure.Ready(stationID, zone.RecoveryRevision) {
			return zone, nil
		}
	}
	return r.state.Activate(zoneID, zone.RecoveryRevision)
}

// Readiness 返回各必须站点的就位情况，并携带目录版本，供恢复页面核对。
func (r *ZoneRecovery) Readiness(zoneID string) map[string]map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	zone, _ := r.state.Get(zoneID)
	result := make(map[string]map[string]bool)
	for _, stationID := range r.catalog.RequiredStationsForZone(zoneID) {
		result[stationID] = map[string]bool{
			"connected":   r.connected[zoneID][stationID],
			"warm":        r.warmup.Ready(stationID, zone.RecoveryRevision),
			"pressurized": r.pressure.Ready(stationID, zone.RecoveryRevision),
		}
	}
	return result
}
