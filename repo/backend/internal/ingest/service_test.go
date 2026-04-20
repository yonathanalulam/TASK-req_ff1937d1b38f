package ingest_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/ingest"
	"github.com/eagle-point/service-portal/internal/models"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// ─── Unit: row-count discrepancy ─────────────────────────────────────────────

func TestRowCount_NoDiscrepancyWithinTolerance(t *testing.T) {
	// 0.1% tolerance: 1000 expected, 999 ingested → within tolerance
	assert.False(t, ingest.HasRowCountDiscrepancy(1000, 999))
}

func TestRowCount_DiscrepancyOutsideTolerance(t *testing.T) {
	// 1000 expected, 990 ingested → 1% discrepancy → outside 0.1% tolerance
	assert.True(t, ingest.HasRowCountDiscrepancy(1000, 990))
}

func TestRowCount_ExpectedZero_IngestedNonzero(t *testing.T) {
	assert.True(t, ingest.HasRowCountDiscrepancy(0, 5))
}

func TestRowCount_ExpectedZero_IngestedZero(t *testing.T) {
	assert.False(t, ingest.HasRowCountDiscrepancy(0, 0))
}

// ─── Unit: schema evolution ──────────────────────────────────────────────────

func TestSchema_AddColumn_NotBreaking(t *testing.T) {
	old := []ingest.SchemaField{{Name: "id", Type: "int64"}}
	new := []ingest.SchemaField{
		{Name: "id", Type: "int64"},
		{Name: "name", Type: "string"},
	}
	assert.False(t, ingest.IsBreakingSchemaChange(old, new))
}

func TestSchema_RemoveColumn_Breaking(t *testing.T) {
	old := []ingest.SchemaField{
		{Name: "id", Type: "int64"},
		{Name: "name", Type: "string"},
	}
	new := []ingest.SchemaField{{Name: "id", Type: "int64"}}
	assert.True(t, ingest.IsBreakingSchemaChange(old, new))
}

func TestSchema_NarrowType_Breaking(t *testing.T) {
	old := []ingest.SchemaField{{Name: "id", Type: "int64"}}
	new := []ingest.SchemaField{{Name: "id", Type: "int32"}}
	assert.True(t, ingest.IsBreakingSchemaChange(old, new))
}

func TestSchema_StringToInt_Breaking(t *testing.T) {
	old := []ingest.SchemaField{{Name: "x", Type: "string"}}
	new := []ingest.SchemaField{{Name: "x", Type: "int"}}
	assert.True(t, ingest.IsBreakingSchemaChange(old, new))
}

func TestSchema_NoChange_NotBreaking(t *testing.T) {
	old := []ingest.SchemaField{{Name: "id", Type: "int64"}}
	new := []ingest.SchemaField{{Name: "id", Type: "int64"}}
	assert.False(t, ingest.IsBreakingSchemaChange(old, new))
}

func TestSchema_HashStable(t *testing.T) {
	a := []ingest.SchemaField{{Name: "a", Type: "string"}, {Name: "b", Type: "int"}}
	b := []ingest.SchemaField{{Name: "b", Type: "int"}, {Name: "a", Type: "string"}}
	assert.Equal(t, ingest.SchemaHash(a), ingest.SchemaHash(b),
		"hash should be order-independent")
}

// ─── Integration: source CRUD ────────────────────────────────────────────────

func setup(t *testing.T) {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"lakehouse_lineage", "lakehouse_metadata",
		"lakehouse_schema_versions", "lakehouse_lifecycle_policies", "legal_holds",
		"ingest_checkpoints", "ingest_jobs", "ingest_sources",
		"login_attempts", "sessions", "user_roles", "user_preferences", "users",
	)
}

func TestSource_CreateAndList(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setup(t)
	svc := ingest.NewService(db, "")

	src, err := svc.CreateSource(context.Background(), ingest.CreateSourceInput{
		Name:       "users_table",
		SourceType: models.SourceDBTable,
		Config:     `{"table":"users"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "users_table", src.Name)
	assert.True(t, src.IsActive)

	all, err := svc.ListSources(context.Background())
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func TestSource_InvalidType(t *testing.T) {
	db := testutil.DBOrSkip(t)
	svc := ingest.NewService(db, "")
	_, err := svc.CreateSource(context.Background(), ingest.CreateSourceInput{
		Name: "x", SourceType: "bogus",
	})
	assert.ErrorIs(t, err, ingest.ErrValidation)
}

// ─── Integration: jobs + checkpoints ─────────────────────────────────────────

func TestJob_CreateAndCheckpointResume(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setup(t)
	svc := ingest.NewService(db, "")
	src, _ := svc.CreateSource(context.Background(), ingest.CreateSourceInput{
		Name: "logs", SourceType: models.SourceLogFile, Config: "{}",
	})

	job, err := svc.CreateJob(context.Background(), src.ID)
	require.NoError(t, err)
	assert.Equal(t, models.JobStatusPending, job.Status)

	require.NoError(t, svc.SaveCheckpoint(context.Background(),
		src.ID, job.ID, models.CheckpointOffset, "1024"))

	// Update with a new value (upsert)
	require.NoError(t, svc.SaveCheckpoint(context.Background(),
		src.ID, job.ID, models.CheckpointOffset, "2048"))

	cp, err := svc.LoadCheckpoint(context.Background(), src.ID, job.ID)
	require.NoError(t, err)
	assert.Equal(t, "2048", cp.CheckpointValue)
}

func TestJob_UpdateProgress(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setup(t)
	svc := ingest.NewService(db, "")
	src, _ := svc.CreateSource(context.Background(), ingest.CreateSourceInput{
		Name: "p", SourceType: models.SourceDBTable, Config: "{}",
	})
	job, _ := svc.CreateJob(context.Background(), src.ID)

	require.NoError(t, svc.UpdateJobProgress(context.Background(), job.ID,
		models.JobStatusRunning, 100, 1000, ""))

	updated, _ := svc.GetJob(context.Background(), job.ID)
	assert.Equal(t, models.JobStatusRunning, updated.Status)
	assert.Equal(t, uint64(100), updated.RowsIngested)
	require.NotNil(t, updated.StartedAt)
}

// ─── Integration: schema versions ────────────────────────────────────────────

func TestRecordSchemaVersion_FirstVersion(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setup(t)
	svc := ingest.NewService(db, "")
	src, _ := svc.CreateSource(context.Background(), ingest.CreateSourceInput{
		Name: "sv", SourceType: models.SourceDBTable, Config: "{}",
	})

	v, err := svc.RecordSchemaVersion(context.Background(), src.ID, []ingest.SchemaField{
		{Name: "id", Type: "int64"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, v.Version)
	assert.False(t, v.IsBreaking)
}

func TestRecordSchemaVersion_BackwardCompatible(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setup(t)
	svc := ingest.NewService(db, "")
	src, _ := svc.CreateSource(context.Background(), ingest.CreateSourceInput{
		Name: "bc", SourceType: models.SourceDBTable, Config: "{}",
	})
	svc.RecordSchemaVersion(context.Background(), src.ID, []ingest.SchemaField{
		{Name: "id", Type: "int64"},
	})
	v2, err := svc.RecordSchemaVersion(context.Background(), src.ID, []ingest.SchemaField{
		{Name: "id", Type: "int64"},
		{Name: "name", Type: "string"},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, v2.Version)
	assert.False(t, v2.IsBreaking)
}

func TestRecordSchemaVersion_BreakingReturnsError(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setup(t)
	svc := ingest.NewService(db, "")
	src, _ := svc.CreateSource(context.Background(), ingest.CreateSourceInput{
		Name: "br", SourceType: models.SourceDBTable, Config: "{}",
	})
	svc.RecordSchemaVersion(context.Background(), src.ID, []ingest.SchemaField{
		{Name: "id", Type: "int64"}, {Name: "name", Type: "string"},
	})
	v2, err := svc.RecordSchemaVersion(context.Background(), src.ID, []ingest.SchemaField{
		{Name: "id", Type: "int64"}, // name removed
	})
	assert.ErrorIs(t, err, ingest.ErrSchemaBroken)
	require.NotNil(t, v2)
	assert.True(t, v2.IsBreaking, "version should be persisted with is_breaking=1")
	assert.Equal(t, 2, v2.Version)
}
