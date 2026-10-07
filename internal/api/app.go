package api

import (
	"context"
	"path/filepath"
	"time"

	"heatpulse/internal/assets"
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
	Catalog              *assets.Catalog
	Events               *store.EventLog
	Audit                *audit.Recorder
	AuditQuery           *audit.Query
	Network              *network.State
	Topology             *network.Topology
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
	// 启动第一步：构建并校验唯一资产目录（供回水管端、安全隔离器件、跨表引用全覆盖）。
	// 校验失败直接返回错误，阻止服务带着残缺拓扑启动。
	catalog, err := assets.Build(assets.DefaultDefinition())
	if err != nil {
		return nil, err
	}

	events := store.NewEventLog(filepath.Join(dataDirectory, "events.jsonl"))
	recorder := audit.NewRecorder(events)

	// 分区运行态、分支拓扑、站点绑定、站点注册表均由同一目录派生，避免拓扑多处各存一份。
	networkState := network.NewState(zonesFromCatalog(catalog))
	topology := network.NewTopology(topologyFromCatalog(catalog))
	if err := topology.Validate(); err != nil {
		return nil, err
	}
	stations := station.NewRegistry(stationsFromCatalog(catalog))

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
	planner := dispatch.NewPlanner(targets, catalog)
	dispatchBuffer := dispatch.NewBuffer()
	stationCommands := station.NewCommandService(stations)
	app := &App{
		DataDirectory: dataDirectory, Catalog: catalog, Events: events, Audit: recorder,
		AuditQuery: audit.NewQuery(events), Network: networkState, Topology: topology,
		Stations: stations, Snapshots: snapshots, Interlock: interlock,
		Pumps: pump.NewService(interlock, recorder), PumpRunner: pumpRunner, Pressure: pressure,
		Valves: valve.NewService(interlock, recorder), ValveOperations: valveOperations,
		Relief: relief, Boundaries: boundaries, Primary: primary,
		Telemetry: telemetry.NewSeries(128), FlowReceipts: flowReceipts,
		Balancer: hydraulic.NewBalancer(0.28), LeakDetector: hydraulic.NewLeakDetector(),
		Protection: protection.NewService(), Latches: protection.NewLatchStore(snapshots),
		Frost: protection.NewFrostProtection(valveOperations), Circulation: circulation, Antifreeze: antifreeze,
		Warmup: warmup, Isolation: isolationService, IsolationCoordinator: isolation.NewCoordinator(isolationService),
		Targets: targets, Planner: planner, DispatchBuffer: dispatchBuffer, StationCommands: stationCommands,
		Manual:      station.NewManualService(valveOperations),
		Maintenance: station.NewMaintenanceService(stations, catalog, snapshots),
	}
	app.SnapshotsBuilder = telemetry.NewSnapshotBuilder(app.Telemetry)
	app.LeakProtection = protection.NewLeakProtection(app.LeakDetector)
	app.ExchangerSwitch = exchanger.NewSwitchService(stations, flowReceipts, primary)
	app.Publisher = dispatch.NewPublisher(planner, targets, catalog, recorder)
	app.Reconnector = recovery.NewReconnector(dispatchBuffer, stations, stationCommands, recorder)
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
