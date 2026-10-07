package network

import (
	"strings"
	"testing"
)

func validCatalog() *Catalog {
	return NewCatalog(
		[]ZoneAsset{{ID: "z1", Name: "一分区"}},
		[]StationAsset{{ID: "st1", Name: "一号站", ZoneID: "z1"}},
		[]ExchangerAsset{{ID: "st1-a", StationID: "st1"}, {ID: "st1-b", StationID: "st1"}},
		[]PumpAsset{{ID: "p1", ZoneID: "z1", StationID: "st1"}},
		[]ValveAsset{
			{ID: "bv-a", Role: ValveBoundary, ZoneID: "z1"},
			{ID: "bv-b", Role: ValveBoundary, ZoneID: "z1"},
			{ID: "bp1", Role: ValveBypass, ZoneID: "z1"},
		},
		[]ReliefAsset{{ID: "rv1", ZoneID: "z1", BranchID: "b1"}},
		[]EndpointAsset{
			{ID: "ep-s", BranchID: "b1", Side: SideSupply},
			{ID: "ep-r", BranchID: "b1", Side: SideReturn},
		},
		[]BranchAsset{{
			ID: "b1", ZoneID: "z1", SupplyEndpoint: "ep-s", ReturnEndpoint: "ep-r",
			BoundaryValves: []string{"bv-a", "bv-b"}, ReliefValve: "rv1",
		}},
	)
}

func TestCatalogValid(t *testing.T) {
	catalog := validCatalog()
	if err := catalog.Validate(); err != nil {
		t.Fatalf("valid catalog rejected: %v", err)
	}
	if catalog.Version() == "" {
		t.Fatal("catalog version must not be empty")
	}
	if got := catalog.Version(); got != validCatalog().Version() {
		t.Fatalf("version not deterministic: %s vs %s", got, validCatalog().Version())
	}
}

func TestCatalogValidationIssues(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(branches *[]BranchAsset, valves *[]ValveAsset, reliefs *[]ReliefAsset, endpoints *[]EndpointAsset)
		want   string
	}{
		{
			name: "missing supply endpoint",
			mutate: func(b *[]BranchAsset, _ *[]ValveAsset, _ *[]ReliefAsset, _ *[]EndpointAsset) {
				(*b)[0].SupplyEndpoint = ""
			},
			want: "supply endpoint is empty",
		},
		{
			name: "supply endpoint on return side",
			mutate: func(b *[]BranchAsset, _ *[]ValveAsset, _ *[]ReliefAsset, e *[]EndpointAsset) {
				for i := range *e {
					if (*e)[i].ID == "ep-s" {
						(*e)[i].Side = SideReturn
					}
				}
			},
			want: "is on return side",
		},
		{
			name: "endpoint belongs to other branch",
			mutate: func(b *[]BranchAsset, _ *[]ValveAsset, _ *[]ReliefAsset, e *[]EndpointAsset) {
				for i := range *e {
					if (*e)[i].ID == "ep-s" {
						(*e)[i].BranchID = "other"
					}
				}
			},
			want: "belongs to branch other",
		},
		{
			name: "same supply and return endpoint",
			mutate: func(b *[]BranchAsset, _ *[]ValveAsset, _ *[]ReliefAsset, _ *[]EndpointAsset) {
				(*b)[0].ReturnEndpoint = "ep-s"
			},
			want: "supply and return endpoints",
		},
		{
			name: "single boundary valve",
			mutate: func(b *[]BranchAsset, _ *[]ValveAsset, _ *[]ReliefAsset, _ *[]EndpointAsset) {
				(*b)[0].BoundaryValves = []string{"bv-a"}
			},
			want: "at least two boundary isolation valves",
		},
		{
			name: "non-boundary valve role",
			mutate: func(b *[]BranchAsset, _ *[]ValveAsset, _ *[]ReliefAsset, _ *[]EndpointAsset) {
				(*b)[0].BoundaryValves = []string{"bv-a", "bp1"}
			},
			want: "expected boundary",
		},
		{
			name: "unknown relief device",
			mutate: func(b *[]BranchAsset, _ *[]ValveAsset, _ *[]ReliefAsset, _ *[]EndpointAsset) {
				(*b)[0].ReliefValve = "ghost"
			},
			want: "references unknown relief device ghost",
		},
		{
			name: "relief bound to other branch",
			mutate: func(b *[]BranchAsset, _ *[]ValveAsset, r *[]ReliefAsset, _ *[]EndpointAsset) {
				(*r)[0].BranchID = "other"
			},
			want: "is bound to branch other",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			catalog := validCatalog()
			branches, valves, reliefs, endpoints := catalog.Branches, catalog.Valves, catalog.Reliefs, catalog.Endpoints
			tc.mutate(&branches, &valves, &reliefs, &endpoints)
			catalog = NewCatalog(catalog.Zones, catalog.Stations, catalog.Exchangers, catalog.Pumps, valves, reliefs, endpoints, branches)
			issues := catalog.ValidationIssues()
			if len(issues) == 0 {
				t.Fatal("expected validation issue, got none")
			}
			found := false
			for _, issue := range issues {
				if issue.AssetID == "b1" && strings.Contains(issue.Message, tc.want) {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected issue containing %q, got %+v", tc.want, issues)
			}
		})
	}
}

func TestCatalogUnknownReferences(t *testing.T) {
	catalog := NewCatalog(
		[]ZoneAsset{{ID: "z1", Name: "一分区"}},
		[]StationAsset{{ID: "st1", Name: "一号站", ZoneID: "nowhere"}},
		[]ExchangerAsset{{ID: "hx1", StationID: "ghost"}},
		[]PumpAsset{{ID: "p1", ZoneID: "nowhere"}},
		[]ValveAsset{{ID: "v1", Role: ValveManual, ZoneID: "nowhere"}},
		nil, nil, nil,
	)
	issues := catalog.ValidationIssues()
	if len(issues) < 4 {
		t.Fatalf("expected at least 4 reference issues, got %+v", issues)
	}
}

func TestCatalogDuplicateIDs(t *testing.T) {
	catalog := NewCatalog(
		[]ZoneAsset{{ID: "shared", Name: "甲"}},
		[]StationAsset{{ID: "shared", Name: "乙站", ZoneID: "shared"}},
		nil, nil, nil, nil, nil, nil,
	)
	if err := catalog.Validate(); err == nil {
		t.Fatal("expected duplicate id rejection")
	}
}

func TestCatalogDerivedViews(t *testing.T) {
	catalog := validCatalog()
	bindings := catalog.StationBindings()
	if len(bindings) != 1 || bindings[0].StationID != "st1" || bindings[0].ZoneID != "z1" || !bindings[0].Required {
		t.Fatalf("unexpected bindings: %+v", bindings)
	}
	sections := catalog.Sections()
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	section := sections[0]
	if section.Upstream != "ep-s" || section.Downstream != "ep-r" || section.ReliefValve != "rv1" {
		t.Fatalf("section not derived from catalog: %+v", section)
	}
	if zone, ok := catalog.StationZone("st1"); !ok || zone != "z1" {
		t.Fatalf("station zone lookup wrong: %q %v", zone, ok)
	}
	if ids := catalog.RequiredStationsForZone("z1"); len(ids) != 1 || ids[0] != "st1" {
		t.Fatalf("required stations wrong: %v", ids)
	}
}
