package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func setupTestApp(t *testing.T) (*App, http.Handler) {
	t.Helper()
	app, err := NewApp(t.TempDir())
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	return app, app.Handler()
}

func decodeData(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode %s: %v", string(body), err)
	}
	return envelope.Data
}

func versionDigest(t *testing.T, data map[string]any) string {
	t.Helper()
	version, ok := data["catalog_version"].(map[string]any)
	if !ok {
		t.Fatalf("response missing catalog_version: %v", data)
	}
	digest, _ := version["digest"].(string)
	return digest
}

func TestCatalogAndOverviewShareVersion(t *testing.T) {
	_, handler := setupTestApp(t)

	catalogReq := httptest.NewRequest(http.MethodGet, "/api/v1/catalog", nil)
	catalogRec := httptest.NewRecorder()
	handler.ServeHTTP(catalogRec, catalogReq)
	if catalogRec.Code != http.StatusOK {
		t.Fatalf("catalog status = %d", catalogRec.Code)
	}
	catalog := decodeData(t, catalogRec.Body.Bytes())
	catalogVersion, _ := catalog["version"].(map[string]any)
	catalogDigest, _ := catalogVersion["digest"].(string)
	if catalogDigest == "" {
		t.Fatal("catalog version digest empty")
	}

	overviewReq := httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil)
	overviewRec := httptest.NewRecorder()
	handler.ServeHTTP(overviewRec, overviewReq)
	overview := decodeData(t, overviewRec.Body.Bytes())
	if got := versionDigest(t, overview); got != catalogDigest {
		t.Fatalf("overview digest %s != catalog digest %s", got, catalogDigest)
	}

	// 联锁快照也必须盖同一目录版本。
	interlocks, _ := overview["interlocks"].(map[string]any)
	north, _ := interlocks["north"].(map[string]any)
	northVersion, _ := north["catalog_version"].(map[string]any)
	if northVersion["digest"] != catalogDigest {
		t.Fatalf("interlock digest %v != %s", northVersion["digest"], catalogDigest)
	}
}

func TestRecoveryCarriesCatalogVersion(t *testing.T) {
	app, handler := setupTestApp(t)
	want := app.Catalog.Version().Digest

	req := httptest.NewRequest(http.MethodPost, "/api/v1/zones/north/recovery", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("begin recovery status = %d body %s", rec.Code, rec.Body.String())
	}
	if got := versionDigest(t, decodeData(t, rec.Body.Bytes())); got != want {
		t.Fatalf("recovery digest %s != %s", got, want)
	}
}

func TestPlanCarriesCatalogVersion(t *testing.T) {
	app, handler := setupTestApp(t)
	want := app.Catalog.Version().Digest

	body := strings.NewReader(`{"targets":{"binhe":70},"delay_millis":0}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dispatch/plans", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create plan status = %d body %s", rec.Code, rec.Body.String())
	}
	data := decodeData(t, rec.Body.Bytes())
	if got := versionDigest(t, data); got != want {
		t.Fatalf("plan digest %s != %s", got, want)
	}
}

func TestPlanRejectsUnknownStation(t *testing.T) {
	_, handler := setupTestApp(t)
	body := strings.NewReader(`{"targets":{"ghost-station":70},"delay_millis":0}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dispatch/plans", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for unknown station, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRecoveryRejectsUnknownZone(t *testing.T) {
	_, handler := setupTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/zones/ghost/recovery", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for unknown zone, got %d", rec.Code)
	}
}
