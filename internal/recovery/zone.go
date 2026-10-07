package recovery

import (
	"fmt"
	"sync"

	"heatpulse/internal/exchanger"
	"heatpulse/internal/network"
	"heatpulse/internal/pump"
)

type ZoneRecovery struct {
	state     *network.State
	catalog   *network.Catalog
	warmup    *exchanger.WarmupService
	pressure  *pump.PressureMonitor
	mu        sync.Mutex
	connected map[string]map[string]bool
}

func NewZoneRecovery(state *network.State, catalog *network.Catalog, warmup *exchanger.WarmupService, pressure *pump.PressureMonitor) *ZoneRecovery {
	return &ZoneRecovery{state: state, catalog: catalog, warmup: warmup, pressure: pressure, connected: make(map[string]map[string]bool)}
}

// CatalogVersion 返回恢复流程引用的资产目录版本。
func (r *ZoneRecovery) CatalogVersion() string {
	return r.catalog.Version()
}

func (r *ZoneRecovery) Begin(zoneID string) (network.Zone, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.knownZone(zoneID) {
		return network.Zone{}, fmt.Errorf("zone %s not present in catalog %s", zoneID, r.catalog.Version())
	}
	r.connected[zoneID] = make(map[string]bool)
	return r.state.Update(zoneID, func(zone *network.Zone) error {
		zone.RecoveryRevision++
		zone.Phase = network.PhaseWarming
		return nil
	})
}

func (r *ZoneRecovery) knownZone(zoneID string) bool {
	for _, zone := range r.catalog.Snapshot().Zones {
		if zone.ID == zoneID {
			return true
		}
	}
	return false
}

func (r *ZoneRecovery) OnConnected(zoneID, stationID string) (network.Zone, error) {
	if !r.knownZone(zoneID) {
		return network.Zone{}, fmt.Errorf("zone %s not present in catalog %s", zoneID, r.catalog.Version())
	}
	if owner, ok := r.catalog.StationZone(stationID); !ok || owner != zoneID {
		return network.Zone{}, fmt.Errorf("station %s is not bound to zone %s in catalog %s", stationID, zoneID, r.catalog.Version())
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
