package store

import (
	"regexp"
	"testing"
)

func TestNewPublicAliasHasSafeDisplayShape(t *testing.T) {
	pattern := regexp.MustCompile(`^[A-Za-z]+ [A-Za-z]+ [A-Za-z]+ [0-9]{6}$`)
	for range 50 {
		alias := newPublicAlias()
		if !pattern.MatchString(alias) {
			t.Fatalf("unexpected public alias %q", alias)
		}
	}
}
