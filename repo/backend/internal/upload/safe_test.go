package upload_test

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/upload"
)

// Minimal byte sequences that http.DetectContentType recognizes. Using these
// instead of real images keeps tests hermetic and fast.
var (
	pngMagic  = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	jpegMagic = []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F'}
	pdfMagic  = []byte("%PDF-1.4\n% test\n")
	gifMagic  = []byte("GIF89a")
	// SVG sniffs as "text/xml; charset=utf-8" — a classic "is this image?" trick
	// that http.DetectContentType will NOT classify as image/*.
	svgPayload = []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
)

// buildMultipartPart wraps body into a *multipart.FileHeader with the given
// declared filename and Content-Type header. Reused by every test case to
// produce the *multipart.FileHeader the code under test expects.
func buildMultipartPart(t *testing.T, filename, contentType string, body []byte) *multipart.FileHeader {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)

	mh := make(textproto.MIMEHeader)
	mh.Set("Content-Disposition",
		`form-data; name="file"; filename="`+filename+`"`)
	if contentType != "" {
		mh.Set("Content-Type", contentType)
	}
	part, err := w.CreatePart(mh)
	require.NoError(t, err)
	_, err = part.Write(body)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	reader := multipart.NewReader(buf, w.Boundary())
	form, err := reader.ReadForm(int64(len(body)) + 10*1024)
	require.NoError(t, err)
	t.Cleanup(func() { _ = form.RemoveAll() })
	require.NotEmpty(t, form.File["file"])
	return form.File["file"][0]
}

// tmpDir gives each test its own scratch directory. httptest's approach of
// using t.TempDir keeps cleanup automatic.
func tmpDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// imageRules is the rule set the review handler uses.
var imageRules = upload.Rules{
	MaxBytes: 5 * 1024 * 1024,
	AllowedMIMEs: map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
	},
}

// attachmentRules is the rule set the ticket handler uses.
var attachmentRules = upload.Rules{
	MaxBytes: 5 * 1024 * 1024,
	AllowedMIMEs: map[string]string{
		"image/jpeg":      ".jpg",
		"image/png":       ".png",
		"application/pdf": ".pdf",
	},
}

// ─── Happy path ──────────────────────────────────────────────────────────────

func TestSave_ValidPNG_WrittenWithDetectedMime(t *testing.T) {
	dir := tmpDir(t)
	// Pad with additional bytes so size > magic header length.
	body := append(append([]byte{}, pngMagic...), bytes.Repeat([]byte{0}, 128)...)
	f := buildMultipartPart(t, "photo.png", "image/png", body)

	res, err := upload.Save(imageRules, f, dir)
	require.NoError(t, err)
	assert.Equal(t, "image/png", res.DetectedMIME)
	assert.Equal(t, "photo.png", res.SanitizedOriginal)
	assert.Equal(t, ".png", filepath.Ext(res.StoredFilename))
	assert.FileExists(t, res.StoragePath)

	// Written bytes exactly match input — no truncation, no extra data.
	got, err := os.ReadFile(res.StoragePath)
	require.NoError(t, err)
	assert.Equal(t, body, got)
}

func TestSave_ValidPDF_AcceptedForAttachments(t *testing.T) {
	dir := tmpDir(t)
	body := append(append([]byte{}, pdfMagic...), bytes.Repeat([]byte{'A'}, 256)...)
	f := buildMultipartPart(t, "contract.pdf", "application/pdf", body)

	res, err := upload.Save(attachmentRules, f, dir)
	require.NoError(t, err)
	assert.Equal(t, "application/pdf", res.DetectedMIME)
	assert.Equal(t, ".pdf", filepath.Ext(res.StoredFilename))
}

// ─── MIME spoofing / mismatch ────────────────────────────────────────────────

func TestSave_PDFClaimingPNG_RejectedByMIMEMismatch(t *testing.T) {
	// Classic bypass: attacker sets Content-Type: image/png, body is a real PDF.
	// The sniffed MIME is application/pdf; rules only allow image/png and
	// image/jpeg. The client also claimed an ALLOWED but different type,
	// which is what triggers ErrMIMEMismatch.
	dir := tmpDir(t)
	body := append(append([]byte{}, pdfMagic...), bytes.Repeat([]byte{0}, 64)...)
	f := buildMultipartPart(t, "evil.png", "image/png", body)

	_, err := upload.Save(imageRules, f, dir)
	require.Error(t, err)
	// The PDF MIME is not in the image rules at all → fails earlier on
	// ErrMIMENotAllowed rather than ErrMIMEMismatch. Either is a correct
	// rejection — assert both paths via ErrorIs.
	assert.True(t,
		errorsIs(err, upload.ErrMIMENotAllowed) || errorsIs(err, upload.ErrMIMEMismatch),
		"expected MIME-not-allowed or MIME-mismatch, got %v", err)

	// No file should be left on disk.
	entries, _ := os.ReadDir(dir)
	assert.Empty(t, entries, "partial file must NOT be persisted on validation failure")
}

func TestSave_SVGClaimingPNG_Rejected(t *testing.T) {
	// SVG with embedded script pretending to be a PNG. Magic-byte detection
	// identifies it as text/xml; not in the image allowlist → rejected.
	dir := tmpDir(t)
	f := buildMultipartPart(t, "x.png", "image/png", svgPayload)

	_, err := upload.Save(imageRules, f, dir)
	require.Error(t, err)
	assert.True(t, errorsIs(err, upload.ErrMIMENotAllowed),
		"SVG must not sneak through the image allowlist (got %v)", err)
}

func TestSave_ExecutableClaimingImage_Rejected(t *testing.T) {
	// Windows PE / ELF headers don't sniff as any allowed MIME.
	dir := tmpDir(t)
	mz := []byte{'M', 'Z', 0x90, 0x00, 0x03, 0x00, 0x00, 0x00}
	body := append(append([]byte{}, mz...), bytes.Repeat([]byte{0}, 128)...)
	f := buildMultipartPart(t, "shell.jpg", "image/jpeg", body)

	_, err := upload.Save(imageRules, f, dir)
	assert.Error(t, err, "PE executable must not pass as image/jpeg")
}

func TestSave_GIFRejected_WhenNotInAllowlist(t *testing.T) {
	// GIF is a valid image format, just not accepted by either of our rules.
	// Without a magic-byte check the server might have accepted it based on
	// the client's Content-Type; with magic checks it's rejected.
	dir := tmpDir(t)
	body := append(append([]byte{}, gifMagic...), bytes.Repeat([]byte{0}, 64)...)
	f := buildMultipartPart(t, "anim.png", "image/png", body)

	_, err := upload.Save(imageRules, f, dir)
	require.Error(t, err)
	assert.True(t, errorsIs(err, upload.ErrMIMENotAllowed) || errorsIs(err, upload.ErrMIMEMismatch))
}

// ─── Size limits ─────────────────────────────────────────────────────────────

func TestSave_DeclaredSizeExceedsLimit_RejectedImmediately(t *testing.T) {
	dir := tmpDir(t)
	rules := upload.Rules{MaxBytes: 32, AllowedMIMEs: imageRules.AllowedMIMEs}
	body := append(append([]byte{}, pngMagic...), bytes.Repeat([]byte{0}, 100)...)
	f := buildMultipartPart(t, "big.png", "image/png", body)

	_, err := upload.Save(rules, f, dir)
	require.Error(t, err)
	assert.True(t, errorsIs(err, upload.ErrDeclaredTooLarge),
		"a declared-size > max must be rejected before opening the file (got %v)", err)
}

// ─── Filename sanitization ───────────────────────────────────────────────────

func TestSanitizeFilename_PathTraversalStripped(t *testing.T) {
	for _, raw := range []string{
		"../../etc/passwd",
		`..\..\windows\system32\cmd.exe`,
		"/absolute/unix/path.png",
		`C:\Users\victim\desktop\file.png`,
	} {
		got, err := upload.SanitizeFilename(raw)
		require.NoError(t, err, "raw=%q", raw)
		assert.NotContains(t, got, "/", "raw=%q sanitized=%q", raw, got)
		assert.NotContains(t, got, `\`, "raw=%q sanitized=%q", raw, got)
		assert.NotContains(t, got, "..", "raw=%q sanitized=%q", raw, got)
	}
}

func TestSanitizeFilename_NullAndControlCharsDropped(t *testing.T) {
	got, err := upload.SanitizeFilename("nasty\x00name\x01\x02.png")
	require.NoError(t, err)
	assert.Equal(t, "nastyname.png", got,
		"null and control bytes must be stripped before persisting")
}

func TestSanitizeFilename_EmptyAfterStripping_Errors(t *testing.T) {
	_, err := upload.SanitizeFilename("\x00\x01\x02")
	assert.ErrorIs(t, err, upload.ErrFilenameInvalid)
}

func TestSanitizeFilename_LongNameTruncatedPreservingExtension(t *testing.T) {
	name := strings.Repeat("a", 400) + ".png"
	got, err := upload.SanitizeFilename(name)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(got), 255)
	assert.True(t, strings.HasSuffix(got, ".png"),
		"truncation must preserve the extension so downloaded files keep their hint")
}

func TestSave_FilenameWithPathSegments_SanitizedBeforePersist(t *testing.T) {
	// End-to-end: the attacker supplies a filename with "../../" segments.
	// After Save returns, SanitizedOriginal must be a plain basename; the
	// stored path must be inside dir (no directory traversal actually
	// occurred). The UUID filename is generated by us so traversal isn't
	// possible via the stored path anyway — this test pins the DB-persisted
	// original name.
	dir := tmpDir(t)
	body := append(append([]byte{}, pngMagic...), bytes.Repeat([]byte{0}, 32)...)
	f := buildMultipartPart(t, "../../etc/passwd.png", "image/png", body)

	res, err := upload.Save(imageRules, f, dir)
	require.NoError(t, err)
	assert.Equal(t, "passwd.png", res.SanitizedOriginal)
	// Stored file path is under dir.
	abs, _ := filepath.Abs(res.StoragePath)
	dirAbs, _ := filepath.Abs(dir)
	assert.True(t, strings.HasPrefix(abs, dirAbs),
		"stored file must live inside the caller-chosen dir, got %q", abs)
}

// ─── Leftover file cleanup on failure ────────────────────────────────────────

func TestSave_OnValidationFailure_NoFileLeftBehind(t *testing.T) {
	dir := tmpDir(t)
	f := buildMultipartPart(t, "bad.png", "image/png", svgPayload)

	_, err := upload.Save(imageRules, f, dir)
	require.Error(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "failed upload must not leave files behind")
}

// errorsIs is a short alias so test lines stay uncluttered.
func errorsIs(err, target error) bool { return errors.Is(err, target) }
