package breaking

import "testing"

func TestInVersionRange(t *testing.T) {
	cases := []struct {
		v, from, to string
		want        bool
	}{
		{"5.0.0", "4.81.0", "5.9.0", true},
		{"5.9.0", "4.81.0", "5.9.0", true},   // upper bound included
		{"4.81.0", "4.81.0", "5.9.0", false}, // lower bound excluded
		{"10.0.0", "9.0.0", "10.1.0", true},  // string order would say false
		{"9.5.0", "10.0.0", "11.0.0", false}, // string order would say true
		{"not-a-version", "1.0.0", "2.0.0", false},
	}
	for _, c := range cases {
		if got := inVersionRange(c.v, c.from, c.to); got != c.want {
			t.Errorf("inVersionRange(%q, %q, %q) = %v, want %v", c.v, c.from, c.to, got, c.want)
		}
	}
}
