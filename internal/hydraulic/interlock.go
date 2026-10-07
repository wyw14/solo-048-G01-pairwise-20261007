package hydraulic

import (
	"fmt"
	"sync"

	"heatpulse/internal/network"
)

type Candidate struct {
	PumpStage  int  `json:"pump_stage"`
	BypassOpen bool `json:"bypass_open"`
}

type Interlock struct {
	state   *network.State
	catalog *network.Catalog
	mu      sync.Mutex
}

func NewInterlock(state *network.State, catalog *network.Catalog) *Interlock {
	return &Interlock{state: state, catalog: catalog, mu: sync.Mutex{}}
}

// CatalogVersion 返回联锁所依据的资产目录版本。
func (i *Interlock) CatalogVersion() string {
	return i.catalog.Version()
}

func (i *Interlock) Snapshot(zoneID string) (Candidate, error) {
	zone, ok := i.state.Get(zoneID)
	if !ok {
		return Candidate{}, fmt.Errorf("zone %s not found", zoneID)
	}
	if _, err := zoneCatalog(i.catalog, zoneID); err != nil {
		return Candidate{}, err
	}
	return Candidate{PumpStage: zone.PumpStage, BypassOpen: zone.BypassOpen}, nil
}

func zoneCatalog(catalog *network.Catalog, zoneID string) (network.ZoneAsset, error) {
	for _, zone := range catalog.Snapshot().Zones {
		if zone.ID == zoneID {
			return zone, nil
		}
	}
	return network.ZoneAsset{}, fmt.Errorf("zone %s not present in catalog %s", zoneID, catalog.Version())
}

func Validate(candidate Candidate) error {
	if candidate.PumpStage < 0 || candidate.PumpStage > 4 {
		return fmt.Errorf("pump stage %d outside operating envelope", candidate.PumpStage)
	}
	if candidate.PumpStage >= 3 && !candidate.BypassOpen {
		return fmt.Errorf("hydraulic interlock rejects stage %d with closed bypass", candidate.PumpStage)
	}
	return nil
}

func (i *Interlock) Transition(zoneID string, change func(Candidate) (Candidate, error)) (network.Zone, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, err := zoneCatalog(i.catalog, zoneID); err != nil {
		return network.Zone{}, err
	}
	return i.state.Update(zoneID, func(zone *network.Zone) error {
		current := Candidate{PumpStage: zone.PumpStage, BypassOpen: zone.BypassOpen}
		candidate, err := change(current)
		if err != nil {
			return err
		}
		if err := Validate(candidate); err != nil {
			return err
		}
		zone.PumpStage = candidate.PumpStage
		zone.BypassOpen = candidate.BypassOpen
		return nil
	})
}

func (i *Interlock) StagePump(zoneID string, stage int) (network.Zone, error) {
	return i.Transition(zoneID, func(candidate Candidate) (Candidate, error) {
		candidate.PumpStage = stage
		return candidate, nil
	})
}

func (i *Interlock) SetBypass(zoneID string, open bool) (network.Zone, error) {
	return i.Transition(zoneID, func(candidate Candidate) (Candidate, error) {
		candidate.BypassOpen = open
		return candidate, nil
	})
}
