package profile

import (
	"fmt"
	"strings"
)

// MaskPhone returns a masked US phone string.
// Input may be any string; only the first 10 decimal digits are considered.
// Result format: "(NXX) ***-XXXX" — area code and last 4 digits are visible.
// Returns "***-****" for any input that does not contain exactly 10 digits.
func MaskPhone(phone string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)
	if len(digits) != 10 {
		return "***-****"
	}
	return fmt.Sprintf("(%s) ***-%s", digits[:3], digits[6:])
}
