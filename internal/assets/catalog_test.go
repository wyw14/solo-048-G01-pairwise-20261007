package assets

import (
	"strings"
	"testing"
)

// validBranchDefinition 返回一份结构完整、可通过校验的定义，测试用例在此基础上做单点破坏。
func validDefinition() Definition {
	return Definition{
		SchemaVersion: 1,
		Zones: []Zone{
			{ID: "z1", Name: "一分区", Branches: []string{"b1"}, Pumps: []string{"p1"}},
		},
		Branches: []Branch{
			{
				ID: "b1", ZoneID: "z1", Name: "一号线",
				SupplyNode: "n-s", ReturnNode: "n-r", Stations: []string{"s1"},
				SupplyBoundary: "bv-s", ReturnBoundary: "bv-r", Relief: "rv1",
			},
		},
		Nodes: []Node{
			{ID: "n-s", BranchID: "b1", Conduit: Supply, Name: "供水端"},
			{ID: "n-r", BranchID: "b1", Conduit: Return, Name: "回水端"},
		},
		Stations: []Station{
			{ID: "s1", Name: "一号站", ZoneID: "z1", BranchID: "b1", Required: true,
				Exchangers: []string{"e1"}, Valves: []string{"v1"}},
		},
		Exchangers: []Exchanger{{ID: "e1", StationID: "s1", Train: "A", Name: "换热器"}},
		Pumps:      []Pump{{ID: "p1", Name: "循环泵", ZoneID: "z1", Stage: 2}},
		Valves:     []Valve{{ID: "v1", Name: "手动阀", StationID: "s1", Role: RoleManual}},
		Boundaries: []BoundaryValve{
			{ID: "bv-s", Name: "供水边界阀", BranchID: "b1", Conduit: Supply},
			{ID: "bv-r", Name: "回水边界阀", BranchID: "b1", Conduit: Return},
		},
		Reliefs: []ReliefDevice{{ID: "rv1", Name: "泄压阀", BranchID: "b1"}},
	}
}

func TestBuildValidCatalog(t *testing.T) {
	catalog, err := Build(validDefinition())
	if err != nil {
		t.Fatalf("expected valid catalog, got %v", err)
	}
	version := catalog.Version()
	if version.Schema != 1 || version.Digest == "" {
		t.Fatalf("unexpected version %+v", version)
	}
	if len(version.Digest) != 12 {
		t.Fatalf("digest should be 12 chars, got %q", version.Digest)
	}
	if got := catalog.RequiredStationsForZone("z1"); len(got) != 1 || got[0] != "s1" {
		t.Fatalf("required stations = %v", got)
	}
	branches := catalog.BranchesForZone("z1")
	if len(branches) != 1 || branches[0].Relief != "rv1" {
		t.Fatalf("branches for zone = %+v", branches)
	}
	if exs := catalog.ExchangersForStation("s1"); len(exs) != 1 || exs[0].Train != "A" {
		t.Fatalf("exchangers = %+v", exs)
	}
	snap := catalog.Snapshot()
	if snap.Version.Digest != version.Digest || len(snap.Branches) != 1 {
		t.Fatalf("snapshot version/content mismatch: %+v", snap.Version)
	}
}

func TestDefaultDefinitionBuilds(t *testing.T) {
	catalog := DefaultCatalog()
	if catalog == nil {
		t.Fatal("default catalog nil")
	}
	for _, branch := range catalog.Branches() {
		if branch.SupplyBoundary == "" || branch.ReturnBoundary == "" || branch.Relief == "" {
			t.Fatalf("default branch %s missing isolation devices", branch.ID)
		}
	}
	if len(catalog.Stations()) != 4 || len(catalog.Zones()) != 3 {
		t.Fatalf("unexpected default catalog size: %d zones %d stations", len(catalog.Zones()), len(catalog.Stations()))
	}
}

func TestDigestStableAndContentSensitive(t *testing.T) {
	base := validDefinition()
	c1, err := Build(base)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := Build(validDefinition())
	if err != nil {
		t.Fatal(err)
	}
	if c1.Version().Digest != c2.Version().Digest {
		t.Fatalf("identical definitions produced different digests: %s vs %s", c1.Version().Digest, c2.Version().Digest)
	}
	changed := validDefinition()
	changed.Pumps[0].Stage = 3
	c3, err := Build(changed)
	if err != nil {
		t.Fatal(err)
	}
	if c1.Version().Digest == c3.Version().Digest {
		t.Fatal("changed pump stage did not change catalog digest")
	}
}

func TestBuildValidationFailures(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Definition)
		want   string
	}{
		{
			name:   "schema version missing",
			mutate: func(d *Definition) { d.SchemaVersion = 0 },
			want:   "schema_version",
		},
		{
			name:   "duplicate global id",
			mutate: func(d *Definition) { d.Pumps[0].ID = "s1" },
			want:   "duplicate id",
		},
		{
			name:   "unknown supply node",
			mutate: func(d *Definition) { d.Branches[0].SupplyNode = "nope" },
			want:   "unknown supply node",
		},
		{
			name:   "supply endpoint on return conduit",
			mutate: func(d *Definition) { d.Nodes[0].Conduit = Return },
			want:   "supply endpoint",
		},
		{
			name:   "endpoint attached to other branch",
			mutate: func(d *Definition) { d.Nodes[0].BranchID = "b-other" },
			want:   "not both attached",
		},
		{
			name:   "missing relief device",
			mutate: func(d *Definition) { d.Branches[0].Relief = "" },
			want:   "lacks relief device",
		},
		{
			name:   "boundary on wrong conduit",
			mutate: func(d *Definition) { d.Boundaries[0].Conduit = Return },
			want:   "supply boundary",
		},
		{
			name:   "same boundary on both conduits",
			mutate: func(d *Definition) { d.Branches[0].ReturnBoundary = "bv-s" },
			want:   "reuses one boundary",
		},
		{
			name:   "relief attached to other branch",
			mutate: func(d *Definition) { d.Reliefs[0].BranchID = "b-other" },
			want:   "relief device",
		},
		{
			name:   "station on unknown branch",
			mutate: func(d *Definition) { d.Stations[0].BranchID = "nope" },
			want:   "attached to branch",
		},
		{
			name:   "station zone disagrees with branch",
			mutate: func(d *Definition) { d.Stations[0].ZoneID = "z9" },
			want:   "disagrees with branch",
		},
		{
			name:   "unknown exchanger reference",
			mutate: func(d *Definition) { d.Stations[0].Exchangers = []string{"nope"} },
			want:   "unknown exchanger",
		},
		{
			name:   "zone with no branch",
			mutate: func(d *Definition) { d.Zones[0].Branches = nil },
			want:   "has no branch",
		},
		{
			name:   "dangling pump",
			mutate: func(d *Definition) { d.Zones[0].Pumps = nil },
			want:   "not assigned to a zone",
		},
		{
			name:   "station without exchanger",
			mutate: func(d *Definition) { d.Stations[0].Exchangers = nil; d.Exchangers = nil },
			want:   "has no exchanger",
		},
		{
			name:   "branch listed under wrong zone",
			mutate: func(d *Definition) { d.Branches[0].ZoneID = "z9" },
			want:   "but listed under",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := validDefinition()
			tc.mutate(&def)
			_, err := Build(def)
			if err == nil {
				t.Fatalf("expected validation error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestBranchMustHaveBothConduitEndpoints(t *testing.T) {
	def := validDefinition()
	// 两根供水管、没有回水管：构建期节点 conduit 自身合法，但分支回水端指向了供水管。
	def.Nodes[1].Conduit = Supply
	_, err := Build(def)
	if err == nil || !strings.Contains(err.Error(), "return endpoint") {
		t.Fatalf("expected return endpoint error, got %v", err)
	}
}
