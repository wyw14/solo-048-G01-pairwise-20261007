package api

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"heatpulse/internal/audit"
	"heatpulse/internal/dispatch"
	"heatpulse/internal/exchanger"
	"heatpulse/internal/hydraulic"
	"heatpulse/internal/isolation"
	"heatpulse/internal/network"
	"heatpulse/internal/protection"
	"heatpulse/internal/pump"
	"heatpulse/internal/recovery"
	"heatpulse/internal/station"
	"heatpulse/internal/store"
	"heatpulse/internal/telemetry"
	"heatpulse/internal/valve"
)

type App struct {
	DataDirectory        string
	Catalog              *network.Catalog
	Events               *store.EventLog
	Audit                *audit.Recorder
	AuditQuery           *audit.Query
	Network              *network.State
	Topology             *network.Topology
	NetworkRegistry      *network.Registry
	Stations             *station.Registry
	Snapshots            *store.StationSnapshotStore
	Interlock            *hydraulic.Interlock
	Pumps                *pump.Service
	PumpRunner           *pump.Runner
	Pressure             *pump.PressureMonitor
	Valves               *valve.Service
	ValveOperations      *valve.Operations
	Relief               *valve.ReliefService
	Boundaries           *valve.BoundaryService
	Primary              *valve.PrimaryService
	Telemetry            *telemetry.Series
	SnapshotsBuilder     *telemetry.SnapshotBuilder
	FlowReceipts         *telemetry.FlowReceipts
	Balancer             *hydraulic.Balancer
	LeakDetector         *hydraulic.LeakDetector
	LeakProtection       *protection.LeakProtection
	Protection           *protection.Service
	Latches              *protection.LatchStore
	Frost                *protection.FrostProtection
	ExchangerSwitch      *exchanger.SwitchService
	Circulation          *exchanger.CirculationRunner
	Antifreeze           *protection.Antifreeze
	Warmup               *exchanger.WarmupService
	Isolation            *isolation.Service
	IsolationCoordinator *isolation.Coordinator
	Targets              *station.TargetService
	Planner              *dispatch.Planner
	Publisher            *dispatch.Publisher
	DispatchBuffer       *dispatch.Buffer
	StationCommands      *station.CommandService
	Reconnector          *recovery.Reconnector
	ZoneRecovery         *recovery.ZoneRecovery
	Manual               *station.ManualService
	Maintenance          *station.MaintenanceService
}

func NewApp(dataDirectory string) (*App, error) {
	return NewAppWithCatalog(dataDirectory, seedCatalog())
}

// NewAppWithCatalog 使用指定资产目录装配应用，便于以不同目录做启动校验测试。
func NewAppWithCatalog(dataDirectory string, catalog *network.Catalog) (*App, error) {
	events := store.NewEventLog(filepath.Join(dataDirectory, "events.jsonl"))
	recorder := audit.NewRecorder(events)
	// 资产目录是恢复、联锁、调度、运维共同引用的唯一数据源；
	// 启动时先校验，校验失败直接中止启动。
	if issues := catalog.ValidationIssues(); len(issues) > 0 {
		causes := make([]error, 0, len(issues))
		for _, issue := range issues {
			causes = append(causes, issue)
		}
		return nil, fmt.Errorf("asset catalog %s failed startup validation: %w", catalog.Version(), errors.Join(causes...))
	}
	zoneSeeds := map[string]network.Zone{
		"north": {Name: "北环", Phase: network.PhaseActive, PumpStage: 2, BypassOpen: true, SupplyPressure: 0.82, ReturnPressure: 0.48},
		"east":  {Name: "东支线", Phase: network.PhaseActive, PumpStage: 2, BypassOpen: true, SupplyPressure: 0.76, ReturnPressure: 0.45},
		"west":  {Name: "西区", Phase: network.PhaseActive, PumpStage: 2, BypassOpen: true, SupplyPressure: 0.79, ReturnPressure: 0.46},
	}
	zones := make([]network.Zone, 0, len(catalog.Snapshot().Zones))
	for _, asset := range catalog.Snapshot().Zones {
		seed := zoneSeeds[asset.ID]
		seed.ID = asset.ID
		if seed.Name == "" {
			seed.Name = asset.Name
		}
		zones = append(zones, seed)
	}
	networkState := network.NewState(zones)
	// 拓扑分支与站点绑定均从目录派生，保证与目录同一版本。
	topology := network.NewTopology(catalog.Sections())
	if err := topology.Validate(); err != nil {
		return nil, fmt.Errorf("asset catalog %s derived topology: %w", catalog.Version(), err)
	}
	targetCelsius := stationTargets()
	stationItems := make([]station.Station, 0, len(catalog.Snapshot().Stations))
	for _, asset := range catalog.Snapshot().Stations {
		exchangers := make(map[string]station.ExchangerPhase, len(catalog.ExchangersForStation(asset.ID)))
		for index, exchangerID := range catalog.ExchangersForStation(asset.ID) {
			if index == 0 {
				exchangers[exchangerID] = station.ExchangerCarrying
			} else {
				exchangers[exchangerID] = station.ExchangerStandby
			}
		}
		stationItems = append(stationItems, station.Station{
			ID: asset.ID, Name: asset.Name, ZoneID: asset.ZoneID, Online: true,
			TargetCelsius: targetCelsius[asset.ID], TargetRevision: 1, ControllerEpoch: 1, Exchangers: exchangers,
		})
	}
	stations := station.NewRegistry(stationItems)
	networkRegistry := network.NewRegistry(catalog.StationBindings())
	snapshots := store.NewStationSnapshotStore(dataDirectory)
	interlock := hydraulic.NewInterlock(networkState, catalog)
	pumpRunner := pump.NewRunner()
	pressure := pump.NewPressureMonitor()
	valveOperations := valve.NewOperations()
	flowReceipts := telemetry.NewFlowReceipts()
	primary := valve.NewPrimaryService()
	circulation := exchanger.NewCirculationRunner(pumpRunner)
	antifreeze := protection.NewAntifreeze(circulation)
	warmup := exchanger.NewWarmupService(stations)
	relief := valve.NewReliefService(2 * time.Millisecond)
	boundaries := valve.NewBoundaryService()
	isolationService := isolation.NewService(topology, networkState, relief, boundaries, recorder)
	targets := station.NewTargetService(stations)
	planner := dispatch.NewPlanner(catalog, targets)
	dispatchBuffer := dispatch.NewBuffer()
	stationCommands := station.NewCommandService(stations)
	app := &App{
		DataDirectory: dataDirectory, Catalog: catalog, Events: events, Audit: recorder, AuditQuery: audit.NewQuery(events),
		Network: networkState, Topology: topology, NetworkRegistry: networkRegistry, Stations: stations,
		Snapshots: snapshots, Interlock: interlock, Pumps: pump.NewService(interlock, recorder),
		PumpRunner: pumpRunner, Pressure: pressure, Valves: valve.NewService(interlock, recorder),
		ValveOperations: valveOperations, Relief: relief, Boundaries: boundaries, Primary: primary,
		Telemetry: telemetry.NewSeries(128), FlowReceipts: flowReceipts,
		Balancer: hydraulic.NewBalancer(0.28), LeakDetector: hydraulic.NewLeakDetector(),
		Protection: protection.NewService(), Latches: protection.NewLatchStore(snapshots),
		Frost: protection.NewFrostProtection(valveOperations), Circulation: circulation, Antifreeze: antifreeze,
		Warmup: warmup, Isolation: isolationService, IsolationCoordinator: isolation.NewCoordinator(isolationService),
		Targets: targets, Planner: planner, DispatchBuffer: dispatchBuffer, StationCommands: stationCommands,
		Manual: station.NewManualService(valveOperations), Maintenance: station.NewMaintenanceService(stations, catalog, snapshots),
	}
	app.SnapshotsBuilder = telemetry.NewSnapshotBuilder(app.Telemetry)
	app.LeakProtection = protection.NewLeakProtection(app.LeakDetector)
	app.ExchangerSwitch = exchanger.NewSwitchService(stations, flowReceipts, primary)
	app.Publisher = dispatch.NewPublisher(planner, targets, recorder)
	app.Reconnector = recovery.NewReconnector(dispatchBuffer, stations, catalog, stationCommands, recorder)
	app.ZoneRecovery = recovery.NewZoneRecovery(networkState, catalog, warmup, pressure)
	if err := app.seed(); err != nil {
		return nil, err
	}
	return app, nil
}

func (a *App) seed() error {
	for _, item := range a.Stations.List() {
		for _, sample := range []telemetry.Sample{
			{StationID: item.ID, Kind: telemetry.KindSupplyPressure, Watermark: 1, Value: 0.78},
			{StationID: item.ID, Kind: telemetry.KindReturnPressure, Watermark: 1, Value: 0.46},
			{StationID: item.ID, Kind: telemetry.KindFlow, Watermark: 1, Value: 8.1},
			{StationID: item.ID, Kind: telemetry.KindValvePosition, Watermark: 1, Value: 72},
			{StationID: item.ID, Kind: telemetry.KindTemperature, Watermark: 1, Value: 58},
		} {
			if err := a.Telemetry.Add(sample); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *App) Close(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		a.Antifreeze.Close()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
