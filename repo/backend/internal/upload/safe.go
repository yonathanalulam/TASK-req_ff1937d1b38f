// Package upload provides a single hardened pathway for writing user-uploaded
// multipart files to disk. It exists because the two places that previously
// handled uploads (ticket attachments and review images) each implemented
// the sequence differently and both trusted the client-supplied Content-Type
// header on the multipart part — an attacker-controlled value.
//
// Three defensive properties this package enforces that a naive
// implementation typically misses:
//
//  1. Magic-byte MIME detection: the first 512 bytes are sniffed with
//     http.DetectContentType and the detected type MUST match the allowlist.
//     The client's declared Content-Type is checked too (so obvious
//     mismatches fail fast with a clear error), but the sniffed type is the
//     authoritative gate. This blocks the classic "SVG with <script> claiming
//     to be image/png" attack.
//
//  2. Bounded streaming writes: io.CopyN with a +1 byte overrun check. The
//     multipart.FileHeader.Size is set by Go's parser from observed bytes,
//     so it's not attacker-controlled — but enforcing the cap at copy time
//     is cheap and prevents future regressions if the caller forgets to
//     bound the request body.
//
//  3. Filename sanitization: SanitizeFilename strips path segments, null
//     bytes, control characters, and caps length. The sanitized value is
//     what callers should persist as "original_name" for display.
package upload

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

// Rules declares what a caller is willing to accept.
type Rules struct {
	// MaxBytes is the maximum permitted file size. Values <= 0 are treated
	// as "no limit" — callers should always set this.
	MaxBytes int64

	// AllowedMIMEs maps an accepted MIME type to the file extension that
	// should be used when persisting files of that type. The map doubles as
	// the allowlist AND as the source of truth for the on-disk extension so
	// the two can never drift. Keys are compared against the sniffed MIME,
	// NOT the client-declared one.
	AllowedMIMEs map[string]string
}

// Result is the outcome of a successful save.
type Result struct {
	StoragePath       string // absolute or repo-relative path on disk
	StoredFilename    string // just the basename (UUID + ext)
	DetectedMIME      string // authoritative, from magic bytes
	SanitizedOriginal string // safe to render and log
	SizeBytes         int64
}

// Sentinel errors — translate to 4xx at the handler layer.
var (
	ErrTooLarge          = errors.New("upload: file exceeds allowed size")
	ErrMIMENotAllowed    = errors.New("upload: detected MIME type is not in the allowlist")
	ErrMIMEMismatch      = errors.New("upload: client-declared MIME does not match detected MIME")
	ErrFilenameInvalid   = errors.New("upload: filename is empty or contains only unsafe characters")
	ErrEmptyFile         = errors.New("upload: file is empty")
	ErrDeclaredTooLarge  = errors.New("upload: declared size exceeds allowed size")
)

// maxFilenameLen caps the *sanitized* original name. 255 is the typical
// file-system limit; DB columns for original_name are usually varchar(255).
const maxFilenameLen = 255

// magicSniffBytes is the length http.DetectContentType inspects. Reading
// more is wasted; reading less blinds the sniffer.
const magicSniffBytes = 512

// Save validates f against rules and writes it under dir as <uuid><ext>.
// The caller is responsible for choosing dir (including any tenant or
// per-entity subpath) so path construction never accepts user input here.
func Save(rules Rules, f *multipart.FileHeader, dir string) (*Result, error) {
	if rules.MaxBytes > 0 && f.Size > rules.MaxBytes {
		// Fast path: the parser already knows the size, no need to open.
		return nil, fmt.Errorf("%w: declared %d > max %d", ErrDeclaredTooLarge, f.Size, rules.MaxBytes)
	}

	src, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("upload: open multipart part: %w", err)
	}
	defer src.Close()

	// Sniff magic bytes from the stream. We buffer them in memory and
	// prepend them back when streaming to disk so the sniffing is transparent
	// to the writer.
	head := make([]byte, magicSniffBytes)
	n, err := io.ReadFull(src, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, fmt.Errorf("upload: read magic bytes: %w", err)
	}
	if n == 0 {
		return nil, ErrEmptyFile
	}
	head = head[:n]

	detected := http.DetectContentType(head)
	// DetectContentType can include a ;charset suffix — strip it.
	if i := strings.Index(detected, ";"); i >= 0 {
		detected = strings.TrimSpace(detected[:i])
	}
	ext, ok := rules.AllowedMIMEs[detected]
	if !ok {
		return nil, fmt.Errorf("%w: detected=%q", ErrMIMENotAllowed, detected)
	}
	// Cross-check the client's claim against the sniffed type. Only warn at
	// this layer: the sniffed type is already authoritative, and some
	// browsers send generic types like application/octet-stream even for
	// known formats. We only reject if the client claimed an ALLOWED but
	// different MIME (which is a clear deception attempt).
	claimed := strings.ToLower(strings.TrimSpace(f.Header.Get("Content-Type")))
	if claimed != "" && claimed != detected {
		if _, clientAlsoAllowed := rules.AllowedMIMEs[claimed]; clientAlsoAllowed {
			return nil, fmt.Errorf("%w: claimed=%q detected=%q", ErrMIMEMismatch, claimed, detected)
		}
	}

	sanitized, err := SanitizeFilename(f.Filename)
	if err != nil {
		return nil, err
	}

	// Ensure the target dir exists. 0o750 instead of 0o755 so group-only
	// read, no other — matches the service uid model.
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("upload: mkdir %q: %w", dir, err)
	}

	stored := uuid.New().String() + ext
	full := filepath.Join(dir, stored)

	dst, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_TRUNC, 0o640)
	if err != nil {
		return nil, fmt.Errorf("upload: create %q: %w", full, err)
	}
	// Remove on error so we don't leak half-written files to disk.
	writeOK := false
	defer func() {
		dst.Close()
		if !writeOK {
			_ = os.Remove(full)
		}
	}()

	// Write the sniffed head first, then the remainder with a +1 byte cap
	// so we fail instead of silently truncating when the source lies.
	if _, err := dst.Write(head); err != nil {
		return nil, fmt.Errorf("upload: write head: %w", err)
	}
	written := int64(n)

	var remaining int64
	if rules.MaxBytes > 0 {
		remaining = rules.MaxBytes - written + 1 // +1 sentinel to detect overrun
	} else {
		remaining = 1 << 62
	}
	m, err := io.CopyN(dst, src, remaining)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("upload: stream body: %w", err)
	}
	written += m

	if rules.MaxBytes > 0 && written > rules.MaxBytes {
		return nil, fmt.Errorf("%w: wrote %d > max %d", ErrTooLarge, written, rules.MaxBytes)
	}

	writeOK = true
	return &Result{
		StoragePath:       full,
		StoredFilename:    stored,
		DetectedMIME:      detected,
		SanitizedOriginal: sanitized,
		SizeBytes:         written,
	}, nil
}

// SanitizeFilename returns a filename safe to display in UIs, log, and
// persist to a varchar column. It:
//
//   - reduces any path segments to the final component (strips ../ and \)
//   - drops null bytes, control chars, and leading/trailing whitespace
//   - collapses runs of whitespace
//   - caps length at maxFilenameLen
//
// Returns ErrFilenameInvalid if nothing usable remains.
func SanitizeFilename(raw string) (string, error) {
	// Reduce path segments. filepath.Base handles both / and \ on the
	// current platform, but we normalize \ → / first so behavior matches
	// on Linux too (where \ is a valid character in a filename).
	s := strings.ReplaceAll(raw, `\`, `/`)
	s = filepath.Base(s)
	if s == "." || s == "/" || s == "" {
		return "", ErrFilenameInvalid
	}

	// Strip null and control characters; keep printable + space.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == 0x00:
			continue // hard drop nulls
		case unicode.IsControl(r):
			continue
		case r == '/', r == '\\':
			continue // any residue from edge cases
		default:
			b.WriteRune(r)
		}
	}
	s = strings.TrimSpace(b.String())

	// Collapse internal whitespace so "foo\t\tbar.png" → "foo bar.png"
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "", ErrFilenameInvalid
	}
	if len(s) > maxFilenameLen {
		// Truncate but preserve extension if we can.
		ext := filepath.Ext(s)
		if ext != "" && len(ext) < 16 {
			keep := maxFilenameLen - len(ext)
			s = s[:keep] + ext
		} else {
			s = s[:maxFilenameLen]
		}
	}
	return s, nil
}
