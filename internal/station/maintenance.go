package station

import (
	"fmt"
	"sync"
	"time"

	"heatpulse/internal/assets"
	"heatpulse/internal/store"
)

type MaintenanceSession struct {
	StationID      string         `json:"station_id"`
	CatalogVersion assets.Version `json:"catalog_version"`
	EnteredAt      time.Time      `json:"entered_at"`
	PreviousTarget float64        `json:"previous_target"`
	PreviousHold   bool           `json:"previous_hold"`
	PreviousOnline bool           `json:"previous_online"`
}

type MaintenanceService struct {
	registry  *Registry
	catalog   *assets.Catalog
	snapshots *store.StationSnapshotStore
	mu        sync.Mutex
	sessions  map[string]MaintenanceSession
}

// NewMaintenanceService 基于唯一资产目录构建运维检修服务：
// 只允许对目录登记的站点进入检修，会话与目录版本绑定，运维页面与其它页面共用同一版本。
func NewMaintenanceService(registry *Registry, catalog *assets.Catalog, snapshots *store.StationSnapshotStore) *MaintenanceService {
	return &MaintenanceService{registry: registry, catalog: catalog, snapshots: snapshots, sessions: make(map[string]MaintenanceSession)}
}

// CatalogVersion 返回运维服务引用的目录版本。
func (s *MaintenanceService) CatalogVersion() assets.Version { return s.catalog.Version() }

func (s *MaintenanceService) Enter(stationID string) (MaintenanceSession, error) {
	if _, ok := s.catalog.Station(stationID); !ok {
		return MaintenanceSession{}, fmt.Errorf("station %s not found in asset catalog", stationID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[stationID]; exists {
		return MaintenanceSession{}, fmt.Errorf("station %s already in maintenance", stationID)
	}
	item, ok := s.registry.Get(stationID)
	if !ok {
		return MaintenanceSession{}, fmt.Errorf("station %s not found", stationID)
	}
	session := MaintenanceSession{
		StationID: stationID, CatalogVersion: s.catalog.Version(), EnteredAt: time.Now().UTC(),
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
