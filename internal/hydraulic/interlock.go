package hydraulic

import (
	"fmt"
	"sync"

	"heatpulse/internal/assets"
	"heatpulse/internal/network"
)

type Candidate struct {
	PumpStage      int            `json:"pump_stage"`
	BypassOpen     bool           `json:"bypass_open"`
	CatalogVersion assets.Version `json:"catalog_version"`
}

type Interlock struct {
	state   *network.State
	catalog *assets.Catalog
	mu      sync.Mutex
}

// NewInterlock 基于网络运行态与唯一资产目录构建联锁：
// 分区身份、挂接泵均以目录为准，联锁输出统一加盖目录版本。
func NewInterlock(state *network.State, catalog *assets.Catalog) *Interlock {
	return &Interlock{state: state, catalog: catalog}
}

// CatalogVersion 返回联锁当前引用的目录版本。
func (i *Interlock) CatalogVersion() assets.Version { return i.catalog.Version() }

func (i *Interlock) Snapshot(zoneID string) (Candidate, error) {
	if _, ok := i.catalog.Zone(zoneID); !ok {
		return Candidate{}, fmt.Errorf("zone %s not found in asset catalog", zoneID)
	}
	zone, ok := i.state.Get(zoneID)
	if !ok {
		return Candidate{}, fmt.Errorf("zone %s not found", zoneID)
	}
	return Candidate{PumpStage: zone.PumpStage, BypassOpen: zone.BypassOpen, CatalogVersion: i.catalog.Version()}, nil
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
	return i.state.Update(zoneID, func(zone *network.Zone) error {
		current := Candidate{PumpStage: zone.PumpStage, BypassOpen: zone.BypassOpen, CatalogVersion: i.catalog.Version()}
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
