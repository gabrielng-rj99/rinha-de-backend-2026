package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"math"
	"os"
)

func main() {
	// Load original references as float64
	f, _ := os.Open("/workspace/resources/references.json.gz")
	gr, _ := gzip.NewReader(f)
	var refs []struct {
		Vector [14]float64 `json:"vector"`
		Label  string      `json:"label"`
	}
	json.NewDecoder(gr).Decode(&refs)
	gr.Close()
	f.Close()

	// Load test data
	data, _ := os.ReadFile("/workspace/test/test-data.json")
	var td struct {
		Entries []struct {
			Request            json.RawMessage `json:"request"`
			ExpectedApproved   bool            `json:"expected_approved"`
			ExpectedFraudScore float64         `json:"expected_fraud_score"`
		} `json:"entries"`
	}
	json.Unmarshal(data, &td)

	// Normalize a test payload using the EXACT same method as the C generator
	// For now, use a simple brute force with float64 distances on the float64 refs
	// to see if the expected results match

	// Actually, we need to normalize the payload to a float64 vector first
	// using the same formulas. Let's use the Go normalizer but convert to float64

	// For entry 0, let's manually compute what the float64 vector should be
	entry := td.Entries[0]
	fmt.Printf("Entry 0 request: %s\n\n", string(entry.Request)[:200])

	// Parse the request to get the raw fields
	var req struct {
		Transaction struct {
			Amount       float64 `json:"amount"`
			Installments int     `json:"installments"`
			RequestedAt  string  `json:"requested_at"`
		} `json:"transaction"`
		Customer struct {
			AvgAmount    float64  `json:"avg_amount"`
			TxCount24h   int      `json:"tx_count_24h"`
			KnownMerchants []string `json:"known_merchants"`
		} `json:"customer"`
		Merchant struct {
			ID        string  `json:"id"`
			MCC       string  `json:"mcc"`
			AvgAmount float64 `json:"avg_amount"`
		} `json:"merchant"`
		Terminal struct {
			IsOnline    bool    `json:"is_online"`
			CardPresent bool    `json:"card_present"`
			KmFromHome  float64 `json:"km_from_home"`
		} `json:"terminal"`
		LastTransaction *struct {
			Timestamp     string  `json:"timestamp"`
			KmFromCurrent float64 `json:"km_from_current"`
		} `json:"last_transaction"`
	}
	json.Unmarshal(entry.Request, &req)

	fmt.Printf("Amount: %f, Installments: %d\n", req.Transaction.Amount, req.Transaction.Installments)
	fmt.Printf("RequestedAt: %s\n", req.Transaction.RequestedAt)
	fmt.Printf("Customer avg: %f, tx_count: %d\n", req.Customer.AvgAmount, req.Customer.TxCount24h)
	fmt.Printf("Merchant: id=%s mcc=%s avg=%f\n", req.Merchant.ID, req.Merchant.MCC, req.Merchant.AvgAmount)
	fmt.Printf("Terminal: online=%v present=%v km=%f\n", req.Terminal.IsOnline, req.Terminal.CardPresent, req.Terminal.KmFromHome)
	fmt.Printf("LastTx: %v\n", req.LastTransaction)

	// Compute the float64 vector (same as C generator would)
	// Using float32 intermediate like C does
	amount_f32 := float32(req.Transaction.Amount)
	installments_f32 := float32(req.Transaction.Installments)
	avg_amount_f32 := float32(req.Customer.AvgAmount)
	tx_count_f32 := float32(req.Customer.TxCount24h)
	km_home_f32 := float32(req.Terminal.KmFromHome)
	merchant_avg_f32 := float32(req.Merchant.AvgAmount)

	var vec [14]float64
	vec[0] = float64(amount_f32 / 10000.0)
	vec[1] = float64(installments_f32 / 12.0)
	if avg_amount_f32 > 0 {
		vec[2] = float64((amount_f32 / avg_amount_f32) / 10.0)
	} else {
		vec[2] = 0
	}
	// Hour and day from timestamp - for now skip, use placeholder
	vec[3] = 0.5 // placeholder
	vec[4] = 0.5 // placeholder
	vec[5] = -1.0
	vec[6] = -1.0
	vec[7] = float64(km_home_f32 / 1000.0)
	vec[8] = float64(tx_count_f32 / 20.0)
	if req.Terminal.IsOnline {
		vec[9] = 1.0
	}
	if req.Terminal.CardPresent {
		vec[10] = 1.0
	}
	unknown := 1.0
	for _, m := range req.Customer.KnownMerchants {
		if m == req.Merchant.ID {
			unknown = 0
			break
		}
	}
	vec[11] = unknown
	vec[12] = 0.15 // mcc 5411
	vec[13] = float64(merchant_avg_f32 / 10000.0)

	// Clamp
	for i := range vec {
		if i == 5 || i == 6 {
			continue // sentinel
		}
		if vec[i] < 0 {
			vec[i] = 0
		}
		if vec[i] > 1 {
			vec[i] = 1
		}
	}

	fmt.Printf("\nFloat64 vector (partial, dims 3,4 are placeholder): %v\n", vec)

	// Now brute force with float64 distances
	type result struct {
		dist  float64
		label string
	}
	top5 := make([]result, 5)
	for i := range top5 {
		top5[i].dist = math.MaxFloat64
	}

	for i, ref := range refs {
		var sum float64
		for d := 0; d < 14; d++ {
			diff := vec[d] - ref.Vector[d]
			sum += diff * diff
		}
		// Find max in top5
		maxIdx := 0
		for j := 1; j < 5; j++ {
			if top5[j].dist > top5[maxIdx].dist {
				maxIdx = j
			}
		}
		if sum < top5[maxIdx].dist {
			top5[maxIdx] = result{sum, ref.Label}
		}
		_ = i
	}

	fraudCount := 0
	fmt.Printf("\nTop 5 nearest (float64 brute force, partial vector):\n")
	for i, r := range top5 {
		fmt.Printf("  %d: dist=%f label=%s\n", i, r.dist, r.label)
		if r.label == "fraud" {
			fraudCount++
		}
	}
	fmt.Printf("Fraud count: %d (expected: %d)\n", fraudCount, int(entry.ExpectedFraudScore*5+0.5))
}
