package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"

	"rinha-backend-2026/internal/index"
	"rinha-backend-2026/internal/vector"
)

// testEntry represents one entry in the test-data.json file.
type testEntry struct {
	Request            json.RawMessage `json:"request"`
	ExpectedApproved   bool            `json:"expected_approved"`
	ExpectedFraudScore float64         `json:"expected_fraud_score"`
}

// testData is the top-level structure of test-data.json.
type testData struct {
	Entries []testEntry `json:"entries"`
}

// discrepancy holds details about a mismatched prediction.
type discrepancy struct {
	Index              int
	Request            string
	ExpectedFraudScore float64
	GotFraudCount      int
	ExpectedApproved   bool
	GotApproved        bool
}

func main() {
	indexPath := flag.String("index", "references.idx", "path to the .idx index file")
	testDataPath := flag.String("testdata", "test-data.json", "path to test-data.json")
	flag.Parse()

	// ── Load index ────────────────────────────────────────────────────
	fmt.Printf("Loading index from %s ...\n", *indexPath)
	idx, err := index.LoadIndex(*indexPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load index: %v\n", err)
		os.Exit(1)
	}
	defer idx.Close()
	fmt.Printf("Index loaded: %d centroids, %d vectors\n", len(idx.Centroids), len(idx.Vectors))

	// ── Read test data ────────────────────────────────────────────────
	fmt.Printf("Reading test data from %s ...\n", *testDataPath)
	data, err := os.ReadFile(*testDataPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read test data: %v\n", err)
		os.Exit(1)
	}

	var td testData
	if err := json.Unmarshal(data, &td); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse test data: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded %d test entries\n", len(td.Entries))

	// ── Run validation ────────────────────────────────────────────────
	total := len(td.Entries)
	var discrepancies []discrepancy

	for i, entry := range td.Entries {
		// Normalize: vector.Normalize takes []byte (raw JSON).
		vec, err := vector.Normalize([]byte(entry.Request))
		if err != nil {
			fmt.Fprintf(os.Stderr, "entry %d: normalize error: %v\n", i, err)
			discrepancies = append(discrepancies, discrepancy{
				Index:              i,
				Request:            string(entry.Request),
				ExpectedFraudScore: entry.ExpectedFraudScore,
				GotFraudCount:      -1,
				ExpectedApproved:   entry.ExpectedApproved,
				GotApproved:        false,
			})
			continue
		}

		// Search the index.
		fraudCount := idx.Search(&vec)

		// The expected_fraud_score is the fraction (fraud_count / 5).
		// Convert back to integer fraud_count for comparison.
		expectedFraudCount := int(math.Round(entry.ExpectedFraudScore * 5.0))

		// Determine approved based on fraud_count.
		// fraud_count 0,1,2 → approved (fraud_score < 0.6)
		// fraud_count 3,4,5 → not approved (fraud_score >= 0.6)
		gotApproved := fraudCount < 3

		// Check for discrepancy.
		if fraudCount != expectedFraudCount || gotApproved != entry.ExpectedApproved {
			discrepancies = append(discrepancies, discrepancy{
				Index:              i,
				Request:            string(entry.Request),
				ExpectedFraudScore: entry.ExpectedFraudScore,
				GotFraudCount:      fraudCount,
				ExpectedApproved:   entry.ExpectedApproved,
				GotApproved:        gotApproved,
			})
		}
	}

	// ── Report ────────────────────────────────────────────────────────
	correct := total - len(discrepancies)
	accuracy := 0.0
	if total > 0 {
		accuracy = float64(correct) / float64(total) * 100.0
	}

	fmt.Println()
	fmt.Println("─── Validation Report ───")
	fmt.Printf("Total tested:     %d\n", total)
	fmt.Printf("Discrepancies:    %d\n", len(discrepancies))
	fmt.Printf("Accuracy:         %.2f%%\n", accuracy)
	fmt.Println()

	if len(discrepancies) > 0 {
		show := 5
		if show > len(discrepancies) {
			show = len(discrepancies)
		}
		fmt.Printf("─── First %d discrepancies ───\n", show)
		for i := 0; i < show; i++ {
			d := discrepancies[i]
			fmt.Printf("  [%d] Entry #%d:\n", i+1, d.Index)
			fmt.Printf("       Request:              %s\n", truncateString(d.Request, 120))
			fmt.Printf("       Expected fraud_score: %.1f (fraud_count=%d)\n", d.ExpectedFraudScore, int(math.Round(d.ExpectedFraudScore*5)))
			fmt.Printf("       Got fraud_count:      %d\n", d.GotFraudCount)
			fmt.Printf("       Expected approved:    %v\n", d.ExpectedApproved)
			fmt.Printf("       Got approved:         %v\n", d.GotApproved)
			fmt.Println()
		}
	}

	fmt.Println("DONE")
}

// truncateString returns s truncated to at most maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
