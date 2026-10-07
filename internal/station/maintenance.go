package station

import (
	"fmt"
	"sync"
	"time"

	"heatpulse/internal/network"
	"heatpulse/internal/store"
)

type MaintenanceSession struct {
	StationID      string    `json:"station_id"`
	CatalogVersion string    `json:"catalog_version"`
	ZoneID         string    `json:"zone_id"`
	EnteredAt      time.Time `json:"entered_at"`
	PreviousTarget float64   `json:"previous_target"`
	PreviousHold   bool      `json:"previous_hold"`
	PreviousOnline bool      `json:"previous_online"`
}

type MaintenanceService struct {
	registry  *Registry
	catalog   *network.Catalog
	snapshots *store.StationSnapshotStore
	mu        sync.Mutex
	sessions  map[string]MaintenanceSession
}

func NewMaintenanceService(registry *Registry, catalog *network.Catalog, snapshots *store.StationSnapshotStore) *MaintenanceService {
	return &MaintenanceService{registry: registry, catalog: catalog, snapshots: snapshots, sessions: make(map[string]MaintenanceSession)}
}

// CatalogVersion 返回运维页面引用的资产目录版本。
func (s *MaintenanceService) CatalogVersion() string {
	return s.catalog.Version()
}

func (s *MaintenanceService) Enter(stationID string) (MaintenanceSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[stationID]; exists {
		return MaintenanceSession{}, fmt.Errorf("station %s already in maintenance", stationID)
	}
	item, ok := s.registry.Get(stationID)
	if !ok {
		return MaintenanceSession{}, fmt.Errorf("station %s not found", stationID)
	}
	zoneID, ok := s.catalog.StationZone(stationID)
	if !ok {
		return MaintenanceSession{}, fmt.Errorf("station %s not present in catalog %s", stationID, s.catalog.Version())
	}
	session := MaintenanceSession{
		StationID: stationID, CatalogVersion: s.catalog.Version(), ZoneID: zoneID, EnteredAt: time.Now().UTC(),
		PreviousTarget: item.TargetCelsius, PreviousHold: item.Hold, PreviousOnline: item.Online,
	}
	updated, err := s.registry.Update(stationID, func(current *Station) error {
		current.Maintenance = true
		current.Hold = true
		return nil
	})
	if err != nil {
		return MaintenanceSession{}, err
	}
	if err := s.snapshots.Put(store.StationSnapshot{
		StationID: stationID, Online: updated.Online, Maintenance: true,
		TargetCelsius: updated.TargetCelsius, ControllerEpoch: updated.ControllerEpoch,
	}); err != nil {
		return MaintenanceSession{}, err
	}
	s.sessions[stationID] = session
	return session, nil
}

func (s *MaintenanceService) Exit(stationID string) (Station, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, exists := s.sessions[stationID]
	if !exists {
		return Station{}, fmt.Errorf("station %s has no maintenance session", stationID)
	}
	updated, err := s.registry.Update(stationID, func(current *Station) error {
		current.Maintenance = false
		current.Hold = session.PreviousHold
		current.TargetCelsius = session.PreviousTarget
		current.Online = session.PreviousOnline
		return nil
	})
	if err != nil {
		return Station{}, err
	}
	delete(s.sessions, stationID)
	return updated, nil
}

func (s *MaintenanceService) Active(stationID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.sessions[stationID]
	return ok
}
