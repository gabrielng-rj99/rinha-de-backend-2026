package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"rinha-backend-2026/internal/index"
	"rinha-backend-2026/internal/vector"
)

func main() {
	idx, _ := index.LoadIndex("data/references.idx")
	defer idx.Close()
	data, _ := os.ReadFile("/workspace/test/test-data.json")
	var td struct {
		Entries []struct {
			Request            json.RawMessage `json:"request"`
			ExpectedApproved   bool            `json:"expected_approved"`
			ExpectedFraudScore float64         `json:"expected_fraud_score"`
		} `json:"entries"`
	}
	json.Unmarshal(data, &td)
	n := len(td.Entries)
	correct := 0
	start := time.Now()
	for i := 0; i < n; i++ {
		vec, _ := vector.Normalize(td.Entries[i].Request)
		fc := idx.Search(&vec)
		app := fc < 3
		efc := int(td.Entries[i].ExpectedFraudScore*5 + 0.5)
		if fc == efc && app == td.Entries[i].ExpectedApproved {
			correct++
		}
	}
	elapsed := time.Since(start)
	fmt.Printf("Tested: %d\n", n)
	fmt.Printf("Correct: %d (%.2f%%)\n", correct, float64(correct)/float64(n)*100)
	fmt.Printf("Time: %v\n", elapsed)
	fmt.Printf("Per request: %v\n", elapsed/time.Duration(n))
	fmt.Printf("Throughput: %.0f req/s\n", float64(n)/elapsed.Seconds())
}
