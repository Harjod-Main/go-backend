package username

import "testing"

func TestNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{in: "abc", want: "abc", wantOK: true},
		{in: "  user.name-1  ", want: "user.name-1", wantOK: true},
		{in: "ab", wantOK: false},
		{in: "", wantOK: false},
		{in: "   ", wantOK: false},
		{in: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantOK: false}, // 31
		{in: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", want: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantOK: true}, // 30
		{in: "ชื่อ", wantOK: false},
		{in: "user name", wantOK: false},
	}
	for _, tt := range tests {
		got, ok := Normalize(tt.in)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("Normalize(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}
