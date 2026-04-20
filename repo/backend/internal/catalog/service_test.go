package catalog_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/catalog"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// ─── Unit: CanModify ──────────────────────────────────────────────────────────

func TestCanModify_Owner(t *testing.T) {
	assert.True(t, catalog.CanModify(10, []string{"service_agent"}, 10))
}

func TestCanModify_Administrator(t *testing.T) {
	assert.True(t, catalog.CanModify(99, []string{"administrator"}, 10))
}

func TestCanModify_NotOwnerNotAdmin(t *testing.T) {
	assert.False(t, catalog.CanModify(5, []string{"service_agent"}, 10))
}

func TestCanModify_RegularUser(t *testing.T) {
	assert.False(t, catalog.CanModify(1, []string{"regular_user"}, 2))
}

func TestCanModify_EmptyRoles(t *testing.T) {
	assert.False(t, catalog.CanModify(1, nil, 2))
}

// ─── Integration: categories ──────────────────────────────────────────────────

func TestCatalog_CreateAndListCategories(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"service_offerings", "service_categories",
	)

	svc := catalog.NewService(db)

	cat, err := svc.CreateCategory(context.Background(), catalog.CreateCategoryInput{
		Name:                  "Plumbing",
		Slug:                  "plumbing",
		Description:           "Pipe and drain services",
		ResponseTimeMinutes:   30,
		CompletionTimeMinutes: 120,
	})
	require.NoError(t, err)
	assert.Equal(t, "Plumbing", cat.Name)
	assert.Equal(t, "plumbing", cat.Slug)
	assert.Equal(t, 30, cat.ResponseTimeMinutes)

	cats, err := svc.ListCategories(context.Background())
	require.NoError(t, err)
	assert.Len(t, cats, 1)
	assert.Equal(t, "Plumbing", cats[0].Name)
}

func TestCatalog_UpdateCategory(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "service_offerings", "service_categories")

	svc := catalog.NewService(db)
	cat, err := svc.CreateCategory(context.Background(), catalog.CreateCategoryInput{
		Name: "Electrical", Slug: "electrical",
	})
	require.NoError(t, err)

	updated, err := svc.UpdateCategory(context.Background(), cat.ID, catalog.CreateCategoryInput{
		Name:        "Electrical Services",
		Description: "Updated desc",
	})
	require.NoError(t, err)
	assert.Equal(t, "Electrical Services", updated.Name)
}

func TestCatalog_DeleteCategory(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "service_offerings", "service_categories")

	svc := catalog.NewService(db)
	cat, _ := svc.CreateCategory(context.Background(), catalog.CreateCategoryInput{
		Name: "Cleaning", Slug: "cleaning",
	})

	err := svc.DeleteCategory(context.Background(), cat.ID)
	require.NoError(t, err)

	// Delete again → ErrNotFound
	err = svc.DeleteCategory(context.Background(), cat.ID)
	assert.ErrorIs(t, err, catalog.ErrNotFound)
}

// ─── Integration: offerings ───────────────────────────────────────────────────

func TestCatalog_CreateAndGetOffering(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"user_browsing_history", "user_favorites",
		"service_offerings", "service_categories",
		"login_attempts", "sessions", "user_roles", "users",
	)

	// Insert agent user
	_, err := db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"agentuser", "agent@test.local", "$2a$04$placeholder", "Agent",
	)
	require.NoError(t, err)
	var agentID uint64
	require.NoError(t, db.QueryRow(`SELECT id FROM users WHERE username='agentuser'`).Scan(&agentID))

	svc := catalog.NewService(db)

	cat, err := svc.CreateCategory(context.Background(), catalog.CreateCategoryInput{
		Name: "Landscaping", Slug: "landscaping",
	})
	require.NoError(t, err)

	offering, err := svc.CreateOffering(context.Background(), agentID, catalog.CreateOfferingInput{
		CategoryID:      cat.ID,
		Name:            "Lawn Mowing",
		Description:     "Weekly lawn care",
		BasePrice:       49.99,
		DurationMinutes: 60,
	})
	require.NoError(t, err)
	assert.Equal(t, "Lawn Mowing", offering.Name)
	assert.Equal(t, agentID, offering.AgentID)
	assert.True(t, offering.ActiveStatus)

	// Get by ID
	got, err := svc.GetOffering(context.Background(), offering.ID)
	require.NoError(t, err)
	assert.Equal(t, offering.ID, got.ID)
	assert.Equal(t, 49.99, got.BasePrice)
}

func TestCatalog_ListOfferings_FilterByCategory(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"user_browsing_history", "user_favorites",
		"service_offerings", "service_categories",
		"login_attempts", "sessions", "user_roles", "users",
	)

	_, _ = db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"filteragent", "filter@test.local", "$2a$04$placeholder", "Filter Agent",
	)
	var agentID uint64
	_ = db.QueryRow(`SELECT id FROM users WHERE username='filteragent'`).Scan(&agentID)

	svc := catalog.NewService(db)
	cat1, _ := svc.CreateCategory(context.Background(), catalog.CreateCategoryInput{Name: "CatA", Slug: "cat-a"})
	cat2, _ := svc.CreateCategory(context.Background(), catalog.CreateCategoryInput{Name: "CatB", Slug: "cat-b"})

	svc.CreateOffering(context.Background(), agentID, catalog.CreateOfferingInput{
		CategoryID: cat1.ID, Name: "Offer A1", DurationMinutes: 30,
	})
	svc.CreateOffering(context.Background(), agentID, catalog.CreateOfferingInput{
		CategoryID: cat1.ID, Name: "Offer A2", DurationMinutes: 30,
	})
	svc.CreateOffering(context.Background(), agentID, catalog.CreateOfferingInput{
		CategoryID: cat2.ID, Name: "Offer B1", DurationMinutes: 30,
	})

	// Filter by cat1 only
	page, err := svc.ListOfferings(context.Background(), catalog.OfferingFilter{
		CategoryID: cat1.ID, Active: -1,
	}, 0, 20)
	require.NoError(t, err)
	assert.Len(t, page.Items, 2)
	for _, o := range page.Items {
		assert.Equal(t, cat1.ID, o.CategoryID)
	}

	// Filter active only
	page, err = svc.ListOfferings(context.Background(), catalog.OfferingFilter{Active: 1}, 0, 20)
	require.NoError(t, err)
	assert.Len(t, page.Items, 3)
}

func TestCatalog_UpdateOffering_ForbiddenForNonOwner(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"user_browsing_history", "user_favorites",
		"service_offerings", "service_categories",
		"login_attempts", "sessions", "user_roles", "users",
	)

	for _, u := range []struct{ user, email string }{
		{"owner", "owner@test.local"},
		{"other", "other@test.local"},
	} {
		db.Exec(
			`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
			u.user, u.email, "$2a$04$placeholder", u.user,
		)
	}
	var ownerID, otherID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='owner'`).Scan(&ownerID)
	db.QueryRow(`SELECT id FROM users WHERE username='other'`).Scan(&otherID)

	svc := catalog.NewService(db)
	cat, _ := svc.CreateCategory(context.Background(), catalog.CreateCategoryInput{
		Name: "Repairs", Slug: "repairs",
	})
	offering, _ := svc.CreateOffering(context.Background(), ownerID, catalog.CreateOfferingInput{
		CategoryID: cat.ID, Name: "Fix It", DurationMinutes: 60,
	})

	// Other user (non-owner, non-admin) cannot update
	_, err := svc.UpdateOffering(context.Background(), offering.ID, otherID, []string{"service_agent"}, catalog.CreateOfferingInput{
		CategoryID: cat.ID, Name: "Hijack", DurationMinutes: 60,
	})
	assert.ErrorIs(t, err, catalog.ErrForbidden)

	// Owner can update
	updated, err := svc.UpdateOffering(context.Background(), offering.ID, ownerID, []string{"service_agent"}, catalog.CreateOfferingInput{
		CategoryID: cat.ID, Name: "Fix It v2", DurationMinutes: 90,
	})
	require.NoError(t, err)
	assert.Equal(t, "Fix It v2", updated.Name)
}

func TestCatalog_ToggleStatus(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"user_browsing_history", "user_favorites",
		"service_offerings", "service_categories",
		"login_attempts", "sessions", "user_roles", "users",
	)

	db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"toggleowner", "toggle@test.local", "$2a$04$placeholder", "Toggle",
	)
	var ownerID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='toggleowner'`).Scan(&ownerID)

	svc := catalog.NewService(db)
	cat, _ := svc.CreateCategory(context.Background(), catalog.CreateCategoryInput{
		Name: "Events", Slug: "events",
	})
	offering, _ := svc.CreateOffering(context.Background(), ownerID, catalog.CreateOfferingInput{
		CategoryID: cat.ID, Name: "Party Plan", DurationMinutes: 120,
	})
	assert.True(t, offering.ActiveStatus)

	// Deactivate
	updated, err := svc.ToggleStatus(context.Background(), offering.ID, ownerID, []string{"service_agent"}, false)
	require.NoError(t, err)
	assert.False(t, updated.ActiveStatus)

	// Filter inactive
	page, err := svc.ListOfferings(context.Background(), catalog.OfferingFilter{Active: 0}, 0, 20)
	require.NoError(t, err)
	assert.Len(t, page.Items, 1)
}

func TestCatalog_GetOffering_NotFound(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "service_offerings", "service_categories")

	svc := catalog.NewService(db)
	_, err := svc.GetOffering(context.Background(), 999999)
	assert.ErrorIs(t, err, catalog.ErrNotFound)
}
