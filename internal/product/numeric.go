package product

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// numericFromString converts a decimal string with at most two fraction digits
// (e.g. "19.99" or "5") into a pgtype.Numeric with scale 2.
func numericFromString(s string) (pgtype.Numeric, error) {
	intPart, frac, _ := strings.Cut(s, ".")
	for len(frac) < 2 {
		frac += "0"
	}
	if len(frac) > 2 {
		return pgtype.Numeric{}, fmt.Errorf("decimal %q has more than 2 fraction digits", s)
	}
	v, ok := new(big.Int).SetString(intPart+frac, 10)
	if !ok {
		return pgtype.Numeric{}, fmt.Errorf("invalid decimal %q", s)
	}
	return pgtype.Numeric{Int: v, Exp: -2, Valid: true}, nil
}

// numericToString renders a pgtype.Numeric as a decimal string with exactly two
// fraction digits, independent of the scale the driver decoded it with.
func numericToString(n pgtype.Numeric) string {
	// InfinityModifier 0 means a finite value.
	if !n.Valid || n.NaN || n.InfinityModifier != 0 || n.Int == nil {
		return "0.00"
	}
	v := new(big.Int).Set(n.Int)
	shift := int(n.Exp) + 2 // value * 10^2 == Int * 10^(Exp+2)
	if shift > 0 {
		v.Mul(v, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(shift)), nil))
	} else if shift < 0 {
		v.Quo(v, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-shift)), nil))
	}
	negative := v.Sign() < 0
	digits := new(big.Int).Abs(v).String()
	for len(digits) < 3 {
		digits = "0" + digits
	}
	out := digits[:len(digits)-2] + "." + digits[len(digits)-2:]
	if negative {
		out = "-" + out
	}
	return out
}
