package testutils

import (
	"testing"
	"time"

	fuzz "github.com/google/gofuzz"
)

func EnsureFuzzCoverage(t *testing.T, expected []string, cb func(t *testing.T, f *fuzz.Fuzzer) string) {
	t.Helper()

	var details []string
	results := make(map[string]bool, len(expected))
	for _, e := range expected {
		results[e] = false
	}
	seed := time.Now().UnixNano()
	f := fuzz.NewWithSeed(seed) // helps with troubleshooting

	defer func() {
		if t.Failed() { // else a bit noisy
			t.Logf("expected to see:  %#v", expected)
			t.Logf("observed results: %#v", results)
			t.Logf("detailed results: %#v", details)
			t.Logf("fuzz seed: %v", seed)
		}
	}()

	for tries := 0; tries < 100; tries++ { // retry a few times if needed
		for i := 0; i < 100; i++ { // always fuzz a moderate amount, don't stop immediately
			res := cb(t, f)
			details = append(details, res)
			if res == "" {
				t.Errorf("invalid empty response from fuzzing callback on iteration %v", (tries*100)+i)
			}
			if _, ok := results[res]; !ok {
				t.Errorf("unrecognized response from fuzzing callback on iteration %v: %v", (tries*100)+i, res)
			}
			if t.Failed() {
				return // already failed either internally or in the callback, either way stop trying
			}
			results[res] = true
		}
		stop := true
		for _, v := range results {
			stop = stop && v
		}
		if stop {
			return // covered all expected values, stop retrying
		}
	}
	missing := make([]string, 0, len(results))
	for _, v := range expected {
		if !results[v] {
			missing = append(missing, v)
		}
	}
	t.Errorf("fuzzy coverage func did not check enough cases after 10k attempts, missing: %#v", missing)
}
