package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskPhone_Standard10Digits(t *testing.T) {
	assert.Equal(t, "(415) ***-1234", MaskPhone("4155551234"))
}

func TestMaskPhone_FormattedInput(t *testing.T) {
	assert.Equal(t, "(415) ***-1234", MaskPhone("(415) 555-1234"))
}

func TestMaskPhone_DashedInput(t *testing.T) {
	assert.Equal(t, "(800) ***-5678", MaskPhone("800-555-5678"))
}

func TestMaskPhone_TooShort(t *testing.T) {
	assert.Equal(t, "***-****", MaskPhone("12345"))
}

func TestMaskPhone_TooLong(t *testing.T) {
	assert.Equal(t, "***-****", MaskPhone("12345678901"))
}

func TestMaskPhone_Empty(t *testing.T) {
	assert.Equal(t, "***-****", MaskPhone(""))
}

func TestMaskPhone_NonDigits(t *testing.T) {
	assert.Equal(t, "***-****", MaskPhone("abcdefghij"))
}

func TestMaskPhone_LeadingZero(t *testing.T) {
	// "0123456789" → area=(012), last4=6789
	assert.Equal(t, "(012) ***-6789", MaskPhone("0123456789"))
}
