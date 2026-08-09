package cmd

import (
	"testing"
	"time"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
)

func planWith(method domain.Method) domain.WritePlan {
	return domain.WritePlan{Fix: domain.Fix{Method: method}}
}

func TestMethodBreakdownOrdersByHowMuchIsInferred(t *testing.T) {
	result := domain.Result{Plans: []domain.WritePlan{
		planWith(domain.MethodClampedEnd),
		planWith(domain.MethodInterpolated),
		planWith(domain.MethodExact),
		planWith(domain.MethodInterpolated),
	}}

	expected := "exact 1, interpolated 2, clamped-end 1"
	if got := methodBreakdown(result); got != expected {
		t.Errorf("methodBreakdown = %q, expected %q", got, expected)
	}
}

func TestMethodBreakdownOmitsAbsentMethods(t *testing.T) {
	result := domain.Result{Plans: []domain.WritePlan{planWith(domain.MethodExact)}}

	if got := methodBreakdown(result); got != "exact 1" {
		t.Errorf("methodBreakdown = %q, expected %q", got, "exact 1")
	}
}

func TestOffsetSuffix(t *testing.T) {
	for _, tc := range []struct {
		offset   time.Duration
		expected string
	}{
		{2 * time.Hour, "+02:00"},
		{-5 * time.Hour, "-05:00"},
		{5*time.Hour + 30*time.Minute, "+05:30"},
		{-(3*time.Hour + 30*time.Minute), "-03:30"},
		{0, "+00:00"},
	} {
		t.Run(tc.expected, func(t *testing.T) {
			got := offsetSuffix(domain.WritePlan{CorrectedOffset: tc.offset})
			if got != tc.expected {
				t.Errorf("offsetSuffix(%s) = %q, expected %q", tc.offset, got, tc.expected)
			}
		})
	}
}

func TestGroupUnlocatedCapsTheList(t *testing.T) {
	unlocated := make([]domain.Unlocated, 25)

	if got := len(groupUnlocated(unlocated)); got != 10 {
		t.Errorf("listed %d entries, expected the list to be capped at 10", got)
	}
}

func TestGroupUnlocatedKeepsShortLists(t *testing.T) {
	unlocated := make([]domain.Unlocated, 3)

	if got := len(groupUnlocated(unlocated)); got != 3 {
		t.Errorf("listed %d entries, expected all 3", got)
	}
}
