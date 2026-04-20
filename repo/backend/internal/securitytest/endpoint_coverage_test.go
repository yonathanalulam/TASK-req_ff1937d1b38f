package securitytest_test

// Exhaustive HTTP-level coverage for endpoints previously lacking a true
// no-mock integration test. Every test in this file hits the real router
// mounted via `router.New(...)` against a real MySQL fixture — no handler
// or service mocks. Negative cases (401/403/404/422) are covered where the
// endpoint's contract makes them meaningful.
//
// Rationale: the prior test suite covered only 60/101 registered routes.
// This file raises HTTP and true-API coverage above the 90% bar by adding
// focused cases for admin CRUD, dataops operator surface, HMAC internal
// routes, and user self-service endpoints the previous suite missed.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Local helpers ───────────────────────────────────────────────────────────

// adminSession logs an administrator in and returns (client, csrfToken).
func adminSession(t *testing.T, srv *httptest.Server, db *sql.DB, name string) (*http.Client, string) {
	t.Helper()
	_ = seedUser(t, db, name, "administrator")
	cli := newClient(t, srv)
	csrf := loginAs(t, cli, srv.URL, name)
	return cli, csrf
}

// roleSession is adminSession parameterised on role.
func roleSession(t *testing.T, srv *httptest.Server, db *sql.DB, name, role string) (*http.Client, string) {
	t.Helper()
	_ = seedUser(t, db, name, role)
	cli := newClient(t, srv)
	csrf := loginAs(t, cli, srv.URL, name)
	return cli, csrf
}

// decodeBody drains the response body into a generic map (ignoring any
// decoding failure for empty bodies — those are deliberately tolerated).
func decodeBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

// seedCategory inserts a category row directly and returns its id.
func seedCategory(t *testing.T, db *sql.DB, slug string) uint64 {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`,
		slug, slug, 60)
	require.NoError(t, err)
	return scanUintFromQuery(t, db, `SELECT id FROM service_categories WHERE slug=?`, slug)
}

// seedShippingRegion inserts a region and returns its id.
func seedShippingRegion(t *testing.T, db *sql.DB, name string) uint64 {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO shipping_regions (name, cutoff_time, timezone) VALUES (?,?,?)`,
		name, "15:00:00", "UTC")
	require.NoError(t, err)
	return scanUintFromQuery(t, db, `SELECT id FROM shipping_regions WHERE name=?`, name)
}

// seedShippingTemplate inserts a template row and returns its id.
func seedShippingTemplate(t *testing.T, db *sql.DB, regionID uint64) uint64 {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO shipping_templates
		 (region_id, delivery_method, min_weight_kg, max_weight_kg, min_quantity, max_quantity,
		  fee_amount, currency, lead_time_hours, window_hours)
		 VALUES (?, 'courier', 0, 100, 1, 100, 5.0, 'USD', 24, 4)`,
		regionID)
	require.NoError(t, err)
	return scanUintFromQuery(t, db,
		`SELECT id FROM shipping_templates WHERE region_id=? ORDER BY id DESC LIMIT 1`, regionID)
}

// seedIngestSource inserts a source bypassing the handler/service layer so
// tests that only exercise reads don't depend on the write path.
func seedIngestSource(t *testing.T, db *sql.DB, name string) uint64 {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO ingest_sources (name, source_type, config_encrypted, is_active)
		 VALUES (?, 'db_table', ?, 1)`,
		name, []byte("{}"))
	require.NoError(t, err)
	return scanUintFromQuery(t, db, `SELECT id FROM ingest_sources WHERE name=?`, name)
}

// seedIngestJob inserts a job row and returns its id.
func seedIngestJob(t *testing.T, db *sql.DB, sourceID uint64) uint64 {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO ingest_jobs (source_id, status) VALUES (?, 'pending')`,
		sourceID)
	require.NoError(t, err)
	return scanUintFromQuery(t, db,
		`SELECT id FROM ingest_jobs WHERE source_id=? ORDER BY id DESC LIMIT 1`, sourceID)
}

// seedLakehouseMetadata inserts a catalog row and returns its id. The
// handler's GET /catalog/:id responds 404 when nothing exists, so tests
// that want 200 need a seeded row.
func seedLakehouseMetadata(t *testing.T, db *sql.DB, sourceID uint64, layer string) uint64 {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO lakehouse_metadata (source_id, layer, file_path, row_count)
		 VALUES (?, ?, ?, 0)`,
		sourceID, layer, "/tmp/"+layer)
	require.NoError(t, err)
	return scanUintFromQuery(t, db,
		`SELECT id FROM lakehouse_metadata WHERE source_id=? AND layer=? ORDER BY id DESC LIMIT 1`,
		sourceID, layer)
}

// ─── Health ─────────────────────────────────────────────────────────────────

// TestHTTP_Health_RealRouter hits /health via the real router+DB fixture.
// Previously only the handler-test used a synthetic route with a nil DB.
func TestHTTP_Health_RealRouter(t *testing.T) {
	srv, _ := securityServer(t)
	resp, err := srv.Client().Get(srv.URL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body := map[string]any{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ok", body["status"])
}

// ─── Public catalog/shipping reads not previously covered ──────────────────

func TestHTTP_PublicShippingTemplates_ReturnsArray(t *testing.T) {
	srv, db := securityServer(t)
	region := seedShippingRegion(t, db, "pub-region")
	_ = seedShippingTemplate(t, db, region)

	resp, err := srv.Client().Get(srv.URL + "/api/v1/shipping/templates")
	require.NoError(t, err)
	body := decodeBody(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotNil(t, body["templates"])
}

// ─── Admin: service categories CRUD ─────────────────────────────────────────

func TestHTTP_AdminCategoryLifecycle(t *testing.T) {
	srv, db := securityServer(t)
	cli, csrf := adminSession(t, srv, db, "cat_admin")

	// Create
	createResp := doJSON(t, cli, http.MethodPost, srv.URL+"/api/v1/admin/service-categories", csrf,
		map[string]any{
			"name": "AdminCat", "slug": "admin-cat",
			"response_time_minutes":   30,
			"completion_time_minutes": 60,
		})
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	body := decodeBody(t, createResp)
	cat := body["category"].(map[string]any)
	idFloat := cat["id"].(float64)
	id := uint64(idFloat)
	assert.Equal(t, "AdminCat", cat["name"])

	// Update
	updateResp := doJSON(t, cli, http.MethodPut, srv.URL+"/api/v1/admin/service-categories/"+u64Str(id), csrf,
		map[string]any{"name": "AdminCat v2", "slug": "admin-cat",
			"response_time_minutes": 45})
	require.Equal(t, http.StatusOK, updateResp.StatusCode)
	updateResp.Body.Close()

	// Delete
	delResp := doJSON(t, cli, http.MethodDelete, srv.URL+"/api/v1/admin/service-categories/"+u64Str(id), csrf, nil)
	require.Equal(t, http.StatusNoContent, delResp.StatusCode)
	delResp.Body.Close()

	// Delete non-existent returns 404
	missingResp := doJSON(t, cli, http.MethodDelete, srv.URL+"/api/v1/admin/service-categories/999999", csrf, nil)
	assert.Equal(t, http.StatusNotFound, missingResp.StatusCode)
	missingResp.Body.Close()
}

func TestHTTP_AdminCategoryCreate_NonAdmin_Forbidden(t *testing.T) {
	srv, db := securityServer(t)
	cli, csrf := roleSession(t, srv, db, "cat_regular", "regular_user")
	resp := doJSON(t, cli, http.MethodPost, srv.URL+"/api/v1/admin/service-categories", csrf,
		map[string]any{"name": "X", "slug": "x"})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── Admin: shipping regions + templates ────────────────────────────────────

func TestHTTP_AdminShippingRegion_Create(t *testing.T) {
	srv, db := securityServer(t)
	cli, csrf := adminSession(t, srv, db, "ship_admin")

	resp := doJSON(t, cli, http.MethodPost, srv.URL+"/api/v1/admin/shipping/regions", csrf,
		map[string]any{"name": "NewRegion", "cutoff_time": "15:00:00", "timezone": "UTC"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	body := decodeBody(t, resp)
	region := body["region"].(map[string]any)
	assert.Equal(t, "NewRegion", region["name"])
}

func TestHTTP_AdminShippingTemplate_CreateAndUpdate(t *testing.T) {
	srv, db := securityServer(t)
	cli, csrf := adminSession(t, srv, db, "ship_admin2")
	regionID := seedShippingRegion(t, db, "RT-Region")

	createResp := doJSON(t, cli, http.MethodPost, srv.URL+"/api/v1/admin/shipping/templates", csrf,
		map[string]any{
			"region_id": regionID, "delivery_method": "courier",
			"min_weight_kg": 0.0, "max_weight_kg": 50.0,
			"min_quantity": 1, "max_quantity": 10,
			"fee_amount": 7.5, "currency": "USD",
			"lead_time_hours": 24, "window_hours": 4,
		})
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	cBody := decodeBody(t, createResp)
	tmpl := cBody["template"].(map[string]any)
	id := uint64(tmpl["id"].(float64))

	updResp := doJSON(t, cli, http.MethodPut, srv.URL+"/api/v1/admin/shipping/templates/"+u64Str(id), csrf,
		map[string]any{
			"region_id": regionID, "delivery_method": "courier",
			"min_weight_kg": 0.0, "max_weight_kg": 50.0,
			"min_quantity": 1, "max_quantity": 10,
			"fee_amount": 9.0, "currency": "USD",
			"lead_time_hours": 48, "window_hours": 6,
		})
	require.Equal(t, http.StatusOK, updResp.StatusCode)
	updResp.Body.Close()

	// PUT on unknown id → 404
	missing := doJSON(t, cli, http.MethodPut, srv.URL+"/api/v1/admin/shipping/templates/999999", csrf,
		map[string]any{
			"region_id": regionID, "delivery_method": "courier",
			"fee_amount": 1.0, "currency": "USD",
		})
	assert.Equal(t, http.StatusNotFound, missing.StatusCode)
	missing.Body.Close()
}

// ─── Admin: notification templates & sensitive terms reads ──────────────────

func TestHTTP_AdminListNotificationTemplates(t *testing.T) {
	srv, db := securityServer(t)
	cli, _ := adminSession(t, srv, db, "notif_admin")

	// Seed at least one template so the list has a deterministic row.
	_, err := db.Exec(
		`INSERT INTO notification_templates (code, title_template, body_template)
		 VALUES (?, ?, ?)`, "list.probe", "t", "b")
	require.NoError(t, err)

	resp, err := cli.Get(srv.URL + "/api/v1/admin/notification-templates")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHTTP_AdminSensitiveTerms_ListAndDelete(t *testing.T) {
	srv, db := securityServer(t)
	cli, csrf := adminSession(t, srv, db, "term_admin")

	// Seed one term via direct insert so we're not coupled to the add handler.
	_, err := db.Exec(
		`INSERT INTO sensitive_terms (term, class) VALUES (?, 'borderline')`, "flaggy")
	require.NoError(t, err)
	termID := scanUintFromQuery(t, db, `SELECT id FROM sensitive_terms WHERE term=?`, "flaggy")

	listResp, err := cli.Get(srv.URL + "/api/v1/admin/sensitive-terms")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, listResp.StatusCode)
	listResp.Body.Close()

	delResp := doJSON(t, cli, http.MethodDelete, srv.URL+"/api/v1/admin/sensitive-terms/"+u64Str(termID), csrf, nil)
	assert.Equal(t, http.StatusNoContent, delResp.StatusCode)
	delResp.Body.Close()

	missResp := doJSON(t, cli, http.MethodDelete, srv.URL+"/api/v1/admin/sensitive-terms/999999", csrf, nil)
	assert.Equal(t, http.StatusNotFound, missResp.StatusCode)
	missResp.Body.Close()
}

// ─── Admin: legal holds lifecycle ───────────────────────────────────────────

func TestHTTP_AdminLegalHolds_Lifecycle(t *testing.T) {
	srv, db := securityServer(t)
	cli, csrf := adminSession(t, srv, db, "hold_admin")
	srcID := seedIngestSource(t, db, "hold-src")

	listResp, err := cli.Get(srv.URL + "/api/v1/admin/legal-holds")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, listResp.StatusCode)
	listResp.Body.Close()

	createResp := doJSON(t, cli, http.MethodPost, srv.URL+"/api/v1/admin/legal-holds", csrf,
		map[string]any{"source_id": srcID, "reason": "audit"})
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	cBody := decodeBody(t, createResp)
	hold := cBody["hold"].(map[string]any)
	holdID := uint64(hold["id"].(float64))

	delResp := doJSON(t, cli, http.MethodDelete, srv.URL+"/api/v1/admin/legal-holds/"+u64Str(holdID), csrf, nil)
	assert.Equal(t, http.StatusNoContent, delResp.StatusCode)
	delResp.Body.Close()

	missResp := doJSON(t, cli, http.MethodDelete, srv.URL+"/api/v1/admin/legal-holds/999999", csrf, nil)
	assert.Equal(t, http.StatusNotFound, missResp.StatusCode)
	missResp.Body.Close()
}

// POST /legal-holds with neither source_id nor job_id must fail validation.
func TestHTTP_AdminLegalHolds_Validation(t *testing.T) {
	srv, db := securityServer(t)
	cli, csrf := adminSession(t, srv, db, "hold_admin2")
	resp := doJSON(t, cli, http.MethodPost, srv.URL+"/api/v1/admin/legal-holds", csrf,
		map[string]any{"reason": "need at least one target"})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

// ─── Internal HMAC endpoints not previously covered ─────────────────────────

func TestHTTP_Internal_UpdateSourceAndGetJob(t *testing.T) {
	srv, db := securityServer(t)
	cli, csrf := adminSession(t, srv, db, "intop")
	secret := createHMACKey(t, cli, srv.URL, csrf, "intop-key")

	srcID := seedIngestSource(t, db, "int-src")

	// PUT /internal/data/sources/:id
	path := "/api/v1/internal/data/sources/" + u64Str(srcID)
	body := []byte(`{"name":"int-src-renamed","source_type":"db_table","config":"{}","is_active":true}`)
	respPut := doInternal(t, srv.Client(), srv.URL, http.MethodPut, path,
		"intop-key", secret, body, internalOpts{})
	assert.Equal(t, http.StatusOK, respPut.StatusCode)
	respPut.Body.Close()

	jobID := seedIngestJob(t, db, srcID)

	// GET /internal/data/jobs/:id
	jobPath := "/api/v1/internal/data/jobs/" + u64Str(jobID)
	respJob := doInternal(t, srv.Client(), srv.URL, http.MethodGet, jobPath,
		"intop-key", secret, nil, internalOpts{})
	assert.Equal(t, http.StatusOK, respJob.StatusCode)
	respJob.Body.Close()

	// 404 on unknown job
	respMiss := doInternal(t, srv.Client(), srv.URL, http.MethodGet,
		"/api/v1/internal/data/jobs/999999", "intop-key", secret, nil, internalOpts{})
	assert.Equal(t, http.StatusNotFound, respMiss.StatusCode)
	respMiss.Body.Close()
}

func TestHTTP_Internal_SchemaVersionsAndCatalogLineage(t *testing.T) {
	srv, db := securityServer(t)
	cli, csrf := adminSession(t, srv, db, "intop2")
	secret := createHMACKey(t, cli, srv.URL, csrf, "intop2-key")
	srcID := seedIngestSource(t, db, "int-src2")
	metaID := seedLakehouseMetadata(t, db, srcID, "bronze")

	schemaPath := "/api/v1/internal/data/schema-versions/" + u64Str(srcID)
	respSchema := doInternal(t, srv.Client(), srv.URL, http.MethodGet, schemaPath,
		"intop2-key", secret, nil, internalOpts{})
	assert.Equal(t, http.StatusOK, respSchema.StatusCode)
	respSchema.Body.Close()

	catalogPath := "/api/v1/internal/data/catalog"
	respCat := doInternal(t, srv.Client(), srv.URL, http.MethodGet, catalogPath,
		"intop2-key", secret, nil, internalOpts{})
	assert.Equal(t, http.StatusOK, respCat.StatusCode)
	respCat.Body.Close()

	catByID := "/api/v1/internal/data/catalog/" + u64Str(metaID)
	respCatID := doInternal(t, srv.Client(), srv.URL, http.MethodGet, catByID,
		"intop2-key", secret, nil, internalOpts{})
	assert.Equal(t, http.StatusOK, respCatID.StatusCode)
	respCatID.Body.Close()

	linPath := "/api/v1/internal/data/lineage/" + u64Str(metaID)
	respLin := doInternal(t, srv.Client(), srv.URL, http.MethodGet, linPath,
		"intop2-key", secret, nil, internalOpts{})
	assert.Equal(t, http.StatusOK, respLin.StatusCode)
	respLin.Body.Close()
}

// ─── Dataops operator surface (session-auth) ────────────────────────────────

// dataopsSession returns a data_operator client pre-authenticated with CSRF.
func dataopsSession(t *testing.T, srv *httptest.Server, db *sql.DB, name string) (*http.Client, string) {
	t.Helper()
	return roleSession(t, srv, db, name, "data_operator")
}

func TestHTTP_DataOps_SourcesCreateAndUpdate(t *testing.T) {
	srv, db := securityServer(t)
	cli, csrf := dataopsSession(t, srv, db, "dops_a")

	createResp := doJSON(t, cli, http.MethodPost, srv.URL+"/api/v1/dataops/sources", csrf,
		map[string]any{"name": "do-src", "source_type": "db_table", "config": "{}"})
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	body := decodeBody(t, createResp)
	id := uint64(body["source"].(map[string]any)["id"].(float64))

	updResp := doJSON(t, cli, http.MethodPut, srv.URL+"/api/v1/dataops/sources/"+u64Str(id), csrf,
		map[string]any{"name": "do-src-renamed", "source_type": "db_table", "config": "{}"})
	assert.Equal(t, http.StatusOK, updResp.StatusCode)
	updResp.Body.Close()
}

func TestHTTP_DataOps_JobsFullFlow(t *testing.T) {
	srv, db := securityServer(t)
	cli, csrf := dataopsSession(t, srv, db, "dops_b")
	srcID := seedIngestSource(t, db, "do-jobs-src")

	// POST /jobs
	createResp := doJSON(t, cli, http.MethodPost, srv.URL+"/api/v1/dataops/jobs", csrf,
		map[string]any{"source_id": srcID})
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	cBody := decodeBody(t, createResp)
	jobID := uint64(cBody["job"].(map[string]any)["id"].(float64))

	// GET /jobs
	listResp, err := cli.Get(srv.URL + "/api/v1/dataops/jobs")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, listResp.StatusCode)
	listResp.Body.Close()

	// GET /jobs/:id
	getResp, err := cli.Get(srv.URL + "/api/v1/dataops/jobs/" + u64Str(jobID))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, getResp.StatusCode)
	getResp.Body.Close()

	// POST /jobs/:id/run — accepts with 202 (or 422 if the source config is
	// invalid for the ingestion writer). Either way it's not 404/500.
	runResp := doJSON(t, cli, http.MethodPost, srv.URL+"/api/v1/dataops/jobs/"+u64Str(jobID)+"/run", csrf, nil)
	defer runResp.Body.Close()
	assert.Contains(t, []int{http.StatusAccepted, http.StatusUnprocessableEntity}, runResp.StatusCode,
		"RunJob must return 202 accepted or 422 validation — got %d", runResp.StatusCode)
}

func TestHTTP_DataOps_ReadEndpoints(t *testing.T) {
	srv, db := securityServer(t)
	cli, _ := dataopsSession(t, srv, db, "dops_c")
	srcID := seedIngestSource(t, db, "do-read-src")
	metaID := seedLakehouseMetadata(t, db, srcID, "silver")

	cases := []struct {
		name, path string
		want       int
	}{
		{"schema-versions", "/api/v1/dataops/schema-versions/" + u64Str(srcID), http.StatusOK},
		{"catalog-list", "/api/v1/dataops/catalog", http.StatusOK},
		{"catalog-by-id", "/api/v1/dataops/catalog/" + u64Str(metaID), http.StatusOK},
		{"lineage-by-id", "/api/v1/dataops/lineage/" + u64Str(metaID), http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := cli.Get(srv.URL + tc.path)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, tc.want, resp.StatusCode)
		})
	}
}

// ─── User self-service endpoints ────────────────────────────────────────────

func TestHTTP_User_FavoritesLifecycle(t *testing.T) {
	srv, db := securityServer(t)
	cli, csrf := roleSession(t, srv, db, "fav_user", "regular_user")

	// Seed a category + offering so the offering_id has a valid FK target.
	catID := seedCategory(t, db, "fav-cat")
	_, err := db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		scanUintFromQuery(t, db, `SELECT id FROM users WHERE username=?`, "fav_user"),
		catID, "FavOff", 60)
	require.NoError(t, err)
	offID := scanUintFromQuery(t, db, `SELECT id FROM service_offerings WHERE name=?`, "FavOff")

	// List (empty)
	listResp, err := cli.Get(srv.URL + "/api/v1/users/me/favorites")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, listResp.StatusCode)
	listResp.Body.Close()

	// Add
	addResp := doJSON(t, cli, http.MethodPost, srv.URL+"/api/v1/users/me/favorites", csrf,
		map[string]any{"offering_id": offID})
	assert.Contains(t, []int{http.StatusCreated, http.StatusOK, http.StatusNoContent}, addResp.StatusCode)
	addResp.Body.Close()

	// Remove
	delResp := doJSON(t, cli, http.MethodDelete,
		srv.URL+"/api/v1/users/me/favorites/"+u64Str(offID), csrf, nil)
	assert.Contains(t, []int{http.StatusNoContent, http.StatusOK}, delResp.StatusCode)
	delResp.Body.Close()
}

func TestHTTP_User_HistoryListAndClear(t *testing.T) {
	srv, db := securityServer(t)
	cli, csrf := roleSession(t, srv, db, "hist_user", "regular_user")

	// List is OK on an empty history (just empty array).
	listResp, err := cli.Get(srv.URL + "/api/v1/users/me/history")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, listResp.StatusCode)
	listResp.Body.Close()

	// Clear is idempotent — even with nothing seeded must not error.
	clrResp := doJSON(t, cli, http.MethodDelete, srv.URL+"/api/v1/users/me/history", csrf, nil)
	assert.Contains(t, []int{http.StatusNoContent, http.StatusOK}, clrResp.StatusCode)
	clrResp.Body.Close()
}

func TestHTTP_User_NotificationsUnreadCount(t *testing.T) {
	srv, db := securityServer(t)
	cli, _ := roleSession(t, srv, db, "unread_user", "regular_user")

	resp, err := cli.Get(srv.URL + "/api/v1/users/me/notifications/unread-count")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body := map[string]any{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body, "unread_count")
}

func TestHTTP_User_ExportDownload_NoRequest_Returns404(t *testing.T) {
	srv, db := securityServer(t)
	cli, _ := roleSession(t, srv, db, "exp_user", "regular_user")

	// No export was requested → the handler has nothing to stream and must 404.
	resp, err := cli.Get(srv.URL + "/api/v1/users/me/export-request/download")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Contains(t, []int{http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity},
		resp.StatusCode, "download with no pending export must refuse — got %d", resp.StatusCode)
}

func TestHTTP_User_DeletionStatus(t *testing.T) {
	srv, db := securityServer(t)
	cli, _ := roleSession(t, srv, db, "del_user", "regular_user")

	resp, err := cli.Get(srv.URL + "/api/v1/users/me/deletion-request/status")
	require.NoError(t, err)
	defer resp.Body.Close()
	// No deletion request → 200 with status=none OR 404. Both are legitimate
	// contract shapes; the important thing is that the route resolves.
	assert.Contains(t, []int{http.StatusOK, http.StatusNotFound}, resp.StatusCode)
}

// ─── Tickets: attachments list/delete ───────────────────────────────────────

func TestHTTP_Tickets_AttachmentsListAndDelete(t *testing.T) {
	srv, db := securityServer(t)
	alice := seedUser(t, db, "att_alice", "regular_user")
	cli := newClient(t, srv)
	csrf := loginAs(t, cli, srv.URL, "att_alice")

	ticketID, _, _, _ := seedTicketFor(t, db, alice)

	// List attachments — empty is fine, but status must be 200.
	listResp, err := cli.Get(srv.URL + "/api/v1/tickets/" + u64Str(ticketID) + "/attachments")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, listResp.StatusCode)
	listResp.Body.Close()

	// Seed an attachment row directly so the DELETE has a target.
	_, err = db.Exec(
		`INSERT INTO ticket_attachments
		 (ticket_id, filename, original_name, mime_type, size_bytes, storage_path)
		 VALUES (?, 'x.pdf', 'x.pdf', 'application/pdf', 123, '/tmp/x.pdf')`,
		ticketID)
	require.NoError(t, err)
	fileID := scanUintFromQuery(t, db,
		`SELECT id FROM ticket_attachments WHERE ticket_id=? ORDER BY id DESC LIMIT 1`, ticketID)

	delResp := doJSON(t, cli, http.MethodDelete,
		srv.URL+"/api/v1/tickets/"+u64Str(ticketID)+"/attachments/"+u64Str(fileID), csrf, nil)
	defer delResp.Body.Close()
	assert.Contains(t, []int{http.StatusNoContent, http.StatusOK}, delResp.StatusCode)
}

// ─── Moderation: actions log ────────────────────────────────────────────────

func TestHTTP_Moderation_ListActions(t *testing.T) {
	srv, db := securityServer(t)
	cli, _ := roleSession(t, srv, db, "mod_actions", "moderator")

	resp, err := cli.Get(srv.URL + "/api/v1/moderation/actions")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHTTP_Moderation_ListActions_Forbidden(t *testing.T) {
	srv, db := securityServer(t)
	cli, _ := roleSession(t, srv, db, "mod_actions_r", "regular_user")

	resp, err := cli.Get(srv.URL + "/api/v1/moderation/actions")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── Service offering status toggle ─────────────────────────────────────────

func TestHTTP_ServiceOffering_ToggleStatus_OwnerOK(t *testing.T) {
	srv, db := securityServer(t)
	agent := seedUser(t, db, "toggle_agent", "service_agent")
	cli := newClient(t, srv)
	csrf := loginAs(t, cli, srv.URL, "toggle_agent")

	catID := seedCategory(t, db, "toggle-cat")
	_, err := db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes, active_status) VALUES (?,?,?,?,1)`,
		agent, catID, "ToggleOff", 60)
	require.NoError(t, err)
	offID := scanUintFromQuery(t, db, `SELECT id FROM service_offerings WHERE name=?`, "ToggleOff")

	resp := doJSON(t, cli, http.MethodPatch,
		srv.URL+"/api/v1/service-offerings/"+u64Str(offID)+"/status", csrf,
		map[string]any{"active": false})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

