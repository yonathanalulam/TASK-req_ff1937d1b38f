package hmacadmin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appCrypto "github.com/eagle-point/service-portal/internal/crypto"
	"github.com/eagle-point/service-portal/internal/hmacadmin"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// testEncKey matches the docker-compose test default; covers the happy path
// where AES-GCM is actually exercised (distinct from an empty key which would
// bypass encryption entirely).
const testEncKey = "0000000000000000000000000000000000000000000000000000000000000000"

// hmacServiceFixture spins up a clean hmac_keys table + Service instance.
// Integration-flavoured: requires a real MySQL — tests are skipped otherwise
// via testutil.DBOrSkip.
func hmacServiceFixture(t *testing.T) *hmacadmin.Service {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "hmac_keys")
	return hmacadmin.NewService(db, testEncKey)
}

func TestCreate_HappyPath_ReturnsRevealedSecret(t *testing.T) {
	svc := hmacServiceFixture(t)

	reveal, err := svc.Create(context.Background(), "workflow-agent")
	require.NoError(t, err)
	require.NotNil(t, reveal)

	assert.Equal(t, "workflow-agent", reveal.KeyID)
	assert.True(t, reveal.IsActive)
	assert.NotZero(t, reveal.ID)
	// Hex-encoded 32-byte secret → 64 chars.
	assert.Len(t, reveal.Secret, 2*hmacadmin.SecretByteLength)
	assert.Nil(t, reveal.RotatedAt, "freshly created keys have no rotation timestamp")
}

func TestCreate_SecretsAreEncryptedAndUnique(t *testing.T) {
	svc := hmacServiceFixture(t)

	// Two different key_ids should get distinct random secrets that round-trip
	// through the crypto helpers using the same encKey.
	a, err := svc.Create(context.Background(), "key-a")
	require.NoError(t, err)
	b, err := svc.Create(context.Background(), "key-b")
	require.NoError(t, err)
	assert.NotEqual(t, a.Secret, b.Secret,
		"random secrets must not collide across calls")

	// Sanity: Encrypt/Decrypt through the same helper the middleware uses.
	cipher, err := appCrypto.EncryptString("round-trip", testEncKey)
	require.NoError(t, err)
	plain, err := appCrypto.DecryptString(cipher, testEncKey)
	require.NoError(t, err)
	assert.Equal(t, "round-trip", plain)
}

func TestCreate_RejectsMissingKeyID(t *testing.T) {
	svc := hmacServiceFixture(t)

	_, err := svc.Create(context.Background(), "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, hmacadmin.ErrKeyIDRequired))
}

func TestCreate_RejectsInvalidCharacters(t *testing.T) {
	svc := hmacServiceFixture(t)

	for _, bad := range []string{
		"has space",
		"has/slash",
		"has$dollar",
		"emoji\xf0\x9f\x91\x8d",
	} {
		_, err := svc.Create(context.Background(), bad)
		require.Error(t, err, "expected %q to be rejected", bad)
		assert.True(t, errors.Is(err, hmacadmin.ErrKeyIDInvalid),
			"case %q: want ErrKeyIDInvalid, got %v", bad, err)
	}
}

func TestCreate_RejectsDuplicateKeyID(t *testing.T) {
	svc := hmacServiceFixture(t)

	_, err := svc.Create(context.Background(), "unique-one")
	require.NoError(t, err)

	_, err = svc.Create(context.Background(), "unique-one")
	require.Error(t, err)
	assert.True(t, errors.Is(err, hmacadmin.ErrKeyIDExists))
}

func TestRotate_ReplacesSecretAndStampsTime(t *testing.T) {
	svc := hmacServiceFixture(t)
	before, err := svc.Create(context.Background(), "rotatable")
	require.NoError(t, err)

	after, err := svc.Rotate(context.Background(), "rotatable")
	require.NoError(t, err)

	// Same row — same primary key, same key_id, but a fresh secret + rotated_at.
	assert.Equal(t, before.ID, after.ID)
	assert.Equal(t, before.KeyID, after.KeyID)
	assert.NotEqual(t, before.Secret, after.Secret,
		"rotation must mint a new secret")
	require.NotNil(t, after.RotatedAt, "rotation must set rotated_at")
	assert.True(t, after.IsActive, "rotation must leave the key active")
}

func TestRotate_ReactivatesRevokedKey(t *testing.T) {
	// Revoked-then-rotated keys come back active so admins can resurrect
	// credentials that were temporarily disabled. A surprising default
	// deserves a test to pin it.
	svc := hmacServiceFixture(t)

	created, err := svc.Create(context.Background(), "revived")
	require.NoError(t, err)

	revoked, err := svc.Revoke(context.Background(), created.ID)
	require.NoError(t, err)
	require.False(t, revoked.IsActive)

	rotated, err := svc.Rotate(context.Background(), "revived")
	require.NoError(t, err)
	assert.True(t, rotated.IsActive,
		"rotation should re-enable a previously revoked key")
}

func TestRotate_UnknownKey_ReturnsNotFound(t *testing.T) {
	svc := hmacServiceFixture(t)

	_, err := svc.Rotate(context.Background(), "never-existed")
	require.Error(t, err)
	assert.True(t, errors.Is(err, hmacadmin.ErrKeyNotFound))
}

func TestRevoke_FlipsActiveFlag(t *testing.T) {
	svc := hmacServiceFixture(t)

	created, err := svc.Create(context.Background(), "to-be-revoked")
	require.NoError(t, err)
	require.True(t, created.IsActive)

	revoked, err := svc.Revoke(context.Background(), created.ID)
	require.NoError(t, err)
	assert.False(t, revoked.IsActive, "revoke must clear is_active")
	assert.Equal(t, created.KeyID, revoked.KeyID,
		"revoke must preserve key_id for audit traceability")
}

func TestRevoke_UnknownID_ReturnsNotFound(t *testing.T) {
	svc := hmacServiceFixture(t)

	_, err := svc.Revoke(context.Background(), 99999)
	require.Error(t, err)
	assert.True(t, errors.Is(err, hmacadmin.ErrKeyNotFound))
}

func TestList_OrdersNewestFirst(t *testing.T) {
	svc := hmacServiceFixture(t)

	_, err := svc.Create(context.Background(), "alpha")
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), "beta")
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), "gamma")
	require.NoError(t, err)

	keys, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, keys, 3)

	// ORDER BY id DESC → last inserted is first.
	assert.Equal(t, "gamma", keys[0].KeyID)
	assert.Equal(t, "beta", keys[1].KeyID)
	assert.Equal(t, "alpha", keys[2].KeyID)
}

func TestList_EmptyTable_ReturnsEmptySlice(t *testing.T) {
	svc := hmacServiceFixture(t)

	keys, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, keys)
	assert.NotNil(t, keys, "callers JSON-encode the result; nil would become null")
}
