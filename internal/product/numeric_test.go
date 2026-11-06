package product

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestNumericFromString(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "19.99", want: "19.99"},
		{in: "5", want: "5.00"},
		{in: "5.5", want: "5.50"},
		{in: "0.00", want: "0.00"},
		{in: "99999999.99", want: "99999999.99"},
		{in: "1.234", wantErr: true},
		{in: "abc", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			n, err := numericFromString(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := numericToString(n); got != tt.want {
				t.Fatalf("round trip of %q = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNumericToStringScales(t *testing.T) {
	tests := []struct {
		name string
		in   pgtype.Numeric
		want string
	}{
		{"scale 0", pgtype.Numeric{Int: bigInt(5), Exp: 0, Valid: true}, "5.00"},
		{"positive exponent", pgtype.Numeric{Int: bigInt(1), Exp: 2, Valid: true}, "100.00"},
		{"scale 2", pgtype.Numeric{Int: bigInt(1999), Exp: -2, Valid: true}, "19.99"},
		{"scale 3 truncates", pgtype.Numeric{Int: bigInt(12345), Exp: -3, Valid: true}, "12.34"},
		{"small value", pgtype.Numeric{Int: bigInt(5), Exp: -2, Valid: true}, "0.05"},
		{"zero", pgtype.Numeric{Int: bigInt(0), Exp: 0, Valid: true}, "0.00"},
		{"invalid", pgtype.Numeric{}, "0.00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := numericToString(tt.in); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidatePrice(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantMsg bool
	}{
		{in: "0", want: "0.00"},
		{in: "10", want: "10.00"},
		{in: "10.5", want: "10.50"},
		{in: "10.55", want: "10.55"},
		{in: "-0.01", wantMsg: true},
		{in: "10.555", wantMsg: true},
		{in: "1e3", wantMsg: true},
		{in: "123456789", wantMsg: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, msg := validatePrice(json.Number(tt.in))
			if tt.wantMsg {
				if msg == "" {
					t.Fatalf("expected a validation message for %q", tt.in)
				}
				return
			}
			if msg != "" || got != tt.want {
				t.Fatalf("validatePrice(%q) = (%q, %q), want (%q, \"\")", tt.in, got, msg, tt.want)
			}
		})
	}
}

func TestValidateSKU(t *testing.T) {
	valid := []string{"ABC123", "LAP-PRO-14", "A", "  X1  "}
	for _, s := range valid {
		if _, msg := validateSKU(s); msg != "" {
			t.Errorf("validateSKU(%q) unexpected message: %s", s, msg)
		}
	}
	invalid := []string{"", "   ", "abc", "LAP_PRO", "-LAP", "LAP-", "LAP--PRO", "LAP PRO", "ÄBC"}
	for _, s := range invalid {
		if _, msg := validateSKU(s); msg == "" {
			t.Errorf("validateSKU(%q) expected a validation message", s)
		}
	}
}

func TestLikeEscaper(t *testing.T) {
	if got := likeEscaper.Replace(`50%_off\now`); got != `50\%\_off\\now` {
		t.Fatalf("unexpected escape result: %q", got)
	}
}
