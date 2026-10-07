package api

import (
	"strings"
	"testing"

	"heatpulse/internal/network"
)

// brokenBranchCatalog 构造一条缺少回水端点且只有一台边界阀的分支，
// 用于验证启动校验必须失败。
func brokenBranchCatalog() *network.Catalog {
	return network.NewCatalog(
		[]network.ZoneAsset{{ID: "z1", Name: "一分区"}},
		[]network.StationAsset{{ID: "st1", Name: "一号站", ZoneID: "z1"}},
		nil, nil,
		[]network.ValveAsset{{ID: "bv-a", Role: network.ValveBoundary, ZoneID: "z1"}},
		[]network.ReliefAsset{{ID: "rv1", ZoneID: "z1", BranchID: "b1"}},
		[]network.EndpointAsset{{ID: "ep-s", BranchID: "b1", Side: network.SideSupply}},
		[]network.BranchAsset{{
			ID: "b1", ZoneID: "z1", StationID: "st1",
			SupplyEndpoint: "ep-s", ReturnEndpoint: "",
			BoundaryValves: []string{"bv-a"}, ReliefValve: "rv1",
		}},
	)
}

func TestNewAppRejectsInvalidCatalog(t *testing.T) {
	_, err := NewAppWithCatalog(t.TempDir(), brokenBranchCatalog())
	if err == nil {
		t.Fatal("application must not start with an invalid asset catalog")
	}
	if !strings.Contains(err.Error(), "failed startup validation") {
		t.Fatalf("error should mention startup validation, got: %v", err)
	}
}

func TestNewAppStartsWithSeedCatalog(t *testing.T) {
	app, err := NewApp(t.TempDir())
	if err != nil {
		t.Fatalf("seed catalog must pass startup validation: %v", err)
	}
	defer app.Close(t.Context())
	if app.Catalog.Version() == "" {
		t.Fatal("assembled app must expose the catalog version")
	}
	if version := app.Planner.CatalogVersion(); version != app.Catalog.Version() {
		t.Fatalf("planner catalog version %s != app version %s", version, app.Catalog.Version())
	}
	if version := app.ZoneRecovery.CatalogVersion(); version != app.Catalog.Version() {
		t.Fatalf("recovery catalog version %s != app version %s", version, app.Catalog.Version())
	}
	if version := app.Maintenance.CatalogVersion(); version != app.Catalog.Version() {
		t.Fatalf("maintenance catalog version %s != app version %s", version, app.Catalog.Version())
	}
	if version := app.Interlock.CatalogVersion(); version != app.Catalog.Version() {
		t.Fatalf("interlock catalog version %s != app version %s", version, app.Catalog.Version())
	}
	if got := len(app.Topology.SectionsForZone("east")); got == 0 {
		t.Fatal("topology must be derived from the catalog (east sections missing)")
	}
}
