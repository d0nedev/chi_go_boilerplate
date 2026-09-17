package product

import (
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestNumericString(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"13000000.00", "13000000.00"},
		{"0.5", "0.5"},
		{"12", "12"},
	}

	for _, tt := range tests {
		var n pgtype.Numeric
		if err := n.Scan(tt.in); err != nil {
			t.Fatal(err)
		}
		if got := numericString(n); got != tt.want {
			t.Errorf("numericString(%s) = %q, want %q", tt.in, got, tt.want)
		}
	}

	if got := numericString(pgtype.Numeric{}); got != "" {
		t.Errorf("invalid numeric = %q, want empty", got)
	}
}

func TestProductRequestValidate(t *testing.T) {
	tests := []struct {
		name  string
		req   ProductRequest
		valid bool
	}{
		{"ok", ProductRequest{"Laptop", "12500.00"}, true},
		{"ok integer price", ProductRequest{"Laptop", "0"}, true},
		{"ok max price", ProductRequest{"Laptop", "9999999999999.99"}, true},
		{"ok 255 multibyte", ProductRequest{strings.Repeat("é", 255), "1"}, true},
		{"blank name", ProductRequest{"   ", "1"}, false},
		{"name too long", ProductRequest{strings.Repeat("a", 256), "1"}, false},
		{"blank price", ProductRequest{"Laptop", ""}, false},
		{"negative", ProductRequest{"Laptop", "-1"}, false},
		{"three decimals", ProductRequest{"Laptop", "1.999"}, false},
		{"overflow numeric(15,2)", ProductRequest{"Laptop", "10000000000000"}, false},
		{"hex", ProductRequest{"Laptop", "270f0000.00"}, false},
		{"exponent", ProductRequest{"Laptop", "1e3"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err == nil) != tt.valid {
				t.Errorf("Validate() = %v, valid = %v", err, tt.valid)
			}
		})
	}
}

func TestCursorRoundTrip(t *testing.T) {
	want := pageCursor{
		CreatedAt: time.Date(2026, 9, 17, 10, 0, 0, 123456000, time.UTC),
		ID:        uuid.MustParse("d90ace71-4deb-4857-966b-2b97a64e6aed"),
	}

	got, err := decodeCursor(want.encode())
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || got.ID != want.ID {
		t.Errorf("got %+v, want %+v", got, want)
	}

	for _, bad := range []string{"!!", "bm9jb2xvbg", "YWJjOmQ5MGFjZTcx"} {
		if _, err := decodeCursor(bad); err == nil {
			t.Errorf("decodeCursor(%q) should fail", bad)
		}
	}
}
