package slug

import "testing"

func TestValidate_valid(t *testing.T) {
	cases := []string{
		"anime",
		"hip-hop",
		"k-pop",
		"abc",
		"foo-bar-baz",
		"a1b2c3",
		"matchup-fans",
	}
	for _, c := range cases {
		if got := Validate(c); got != "" {
			t.Errorf("Validate(%q) = %q, want empty (valid)", c, got)
		}
	}
}

func TestValidate_invalid(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ReasonInvalid},                  // too short
		{"ab", ReasonInvalid},                // shorter than MinLength
		{"-leading-hyphen", ReasonInvalid},   // bad pattern
		{"trailing-hyphen-", ReasonInvalid},  // bad pattern
		{"double--hyphen", ReasonInvalid},    // bad pattern
		{"UPPER", ReasonInvalid},             // not normalized — Validate is strict, callers Normalize first
		{"has space", ReasonInvalid},         // whitespace
		{"emoji-🎉", ReasonInvalid},           // non-ascii
		{"this-slug-is-way-way-way-too-long-for-the-max", ReasonInvalid}, // too long
	}
	for _, c := range cases {
		if got := Validate(c.in); got != c.want {
			t.Errorf("Validate(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidate_reserved(t *testing.T) {
	cases := []string{
		"new", "admin", "api", "settings", "members", "matchup", "communities",
	}
	for _, c := range cases {
		if got := Validate(c); got != ReasonReserved {
			t.Errorf("Validate(%q) = %q, want %q", c, got, ReasonReserved)
		}
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  Anime  ", "anime"},
		{"K-POP", "k-pop"},
		{"\thip-hop\n", "hip-hop"},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
