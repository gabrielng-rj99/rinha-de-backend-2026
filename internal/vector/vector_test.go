package vector

import (
	"encoding/json"
	"testing"
)

func TestNormalizeBasic(t *testing.T) {
	payload := `{
		"transaction": {
			"amount": 5000.00,
			"installments": 3,
			"requested_at": "2026-01-15T14:30:00Z"
		},
		"customer": {
			"avg_amount": 200.00,
			"tx_count_24h": 5,
			"known_merchants": ["m001", "m002"]
		},
		"terminal": {
			"km_from_home": 15.0,
			"is_online": true,
			"card_present": false
		},
		"merchant": {
			"id": "m001",
			"mcc": "5411",
			"avg_amount": 300.00
		},
		"last_transaction": {
			"timestamp": "2026-01-15T14:00:00Z",
			"km_from_current": 50.0
		}
	}`

	vec, err := Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	if vec[0] < 0 || vec[0] > 1.0 {
		t.Errorf("dim 0 (amount) out of range: %f", vec[0])
	}
	if vec[9] != 1.0 {
		t.Errorf("dim 9 (is_online) expected 1.0, got %f", vec[9])
	}
	if vec[10] != 0.0 {
		t.Errorf("dim 10 (card_present) expected 0.0, got %f", vec[10])
	}
	if vec[11] != 0.0 {
		t.Errorf("dim 11 (unknown_merchant) expected 0.0 (merchant known), got %f", vec[11])
	}
	if vec[5] == -1.0 {
		t.Errorf("dim 5 should not be sentinel when last_transaction present")
	}
	if vec[6] == -1.0 {
		t.Errorf("dim 6 should not be sentinel when last_transaction present")
	}
}

func TestNormalizeNullLastTx(t *testing.T) {
	payload := `{
		"transaction": {
			"amount": 100.00,
			"installments": 1,
			"requested_at": "2026-06-01T10:00:00Z"
		},
		"customer": {
			"avg_amount": 50.00,
			"tx_count_24h": 0,
			"known_merchants": ["m001"]
		},
		"terminal": {
			"km_from_home": 0.0,
			"is_online": false,
			"card_present": true
		},
		"merchant": {
			"id": "unknown_99",
			"mcc": "9999",
			"avg_amount": 0.0
		},
		"last_transaction": null
	}`

	vec, err := Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	if vec[5] != -1.0 {
		t.Errorf("dim 5 expected sentinel -1.0, got %f", vec[5])
	}
	if vec[6] != -1.0 {
		t.Errorf("dim 6 expected sentinel -1.0, got %f", vec[6])
	}
	if vec[11] != 1.0 {
		t.Errorf("dim 11 expected 1.0 (unknown merchant), got %f", vec[11])
	}
	if vec[12] != 0.5 {
		t.Errorf("dim 12 expected 0.5 (default risk 0.5), got %f", vec[12])
	}
}

func TestDistSquared(t *testing.T) {
	a := &[14]float32{1.0, 0.0, 0.5, 0.3, 0.1, 0.0, 0.0, 0.2, 0.25, 1.0, 0.0, 0.0, 0.5, 0.1}
	b := &[14]float32{1.0, 0.0, 0.5, 0.3, 0.1, 0.0, 0.0, 0.2, 0.25, 1.0, 0.0, 0.0, 0.5, 0.1}

	d := DistSquared(a, b)
	if d != 0.0 {
		t.Errorf("identical vectors: expected 0, got %f", d)
	}

	c := &[14]float32{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	d = DistSquared(a, c)
	expected := 2.7125
	if d < expected-1e-4 || d > expected+1e-4 {
		t.Errorf("squared distance: expected ~%f, got %f", expected, d)
	}
}

func TestMccRisk(t *testing.T) {
	if r := MccRisk("5411"); r != 0.15 {
		t.Errorf("5411: expected 0.15, got %f", r)
	}
	if r := MccRisk("9999"); r != 0.5 {
		t.Errorf("9999: expected 0.5 (default), got %f", r)
	}
}

func TestNormalizeClampBoundaries(t *testing.T) {
	payload := `{
		"transaction": {
			"amount": 99999.99,
			"installments": 99,
			"requested_at": "2026-01-15T14:30:00Z"
		},
		"customer": {
			"avg_amount": 0.0,
			"tx_count_24h": 999,
			"known_merchants": []
		},
		"terminal": {
			"km_from_home": 99999.0,
			"is_online": false,
			"card_present": false
		},
		"merchant": {
			"id": "x",
			"mcc": "5411",
			"avg_amount": 99999.0
		},
		"last_transaction": null
	}`

	vec, err := Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	for i, v := range vec {
		if v > 1.0 {
			t.Errorf("dim %d exceeds max value 1.0: %f", i, v)
		}
	}
	if vec[0] != 1.0 {
		t.Errorf("dim 0 (amount) expected 1.0, got %f", vec[0])
	}
	if vec[1] != 1.0 {
		t.Errorf("dim 1 (installments) expected 1.0, got %f", vec[1])
	}
	if vec[2] != 1.0 {
		t.Errorf("dim 2 (amount_vs_avg) expected 1.0, got %f", vec[2])
	}
	if vec[7] != 1.0 {
		t.Errorf("dim 7 (km_from_home) expected 1.0, got %f", vec[7])
	}
	if vec[8] != 1.0 {
		t.Errorf("dim 8 (tx_count_24h) expected 1.0, got %f", vec[8])
	}
	if vec[13] != 1.0 {
		t.Errorf("dim 13 (merchant_avg_amount) expected 1.0, got %f", vec[13])
	}
}

func TestNormalizeDayOfWeek(t *testing.T) {
	// 2026-01-15 is a Thursday
	payload := `{
		"transaction": {
			"amount": 100,
			"installments": 1,
			"requested_at": "2026-01-15T14:30:00Z"
		},
		"customer": {"avg_amount": 100, "tx_count_24h": 0, "known_merchants": ["x"]},
		"terminal": {"km_from_home": 0, "is_online": false, "card_present": false},
		"merchant": {"id": "x", "mcc": "0000", "avg_amount": 0},
		"last_transaction": null
	}`
	vec, err := Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}
	// Thursday = 3 (Mon=0, Tue=1, Wed=2, Thu=3)
	// dim 4 = 3/6 = 0.5
	if vec[4] != 0.5 {
		t.Errorf("dim 4 (day_of_week for Thu) expected 0.5, got %f", vec[4])
	}
}

func TestJSONRoundTripStruct(t *testing.T) {
	payload := `{"transaction":{"amount":150.50,"installments":2,"requested_at":"2026-03-10T08:15:00Z"},"customer":{"avg_amount":75.0,"tx_count_24h":3,"known_merchants":["m001","m002"]},"terminal":{"km_from_home":5.2,"is_online":true,"card_present":true},"merchant":{"id":"m999","mcc":"5812","avg_amount":180.0},"last_transaction":null}`

	var tp TransactionPayload
	if err := json.Unmarshal([]byte(payload), &tp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if tp.Transaction.Amount != 150.50 {
		t.Errorf("Amount: expected 150.50, got %f", tp.Transaction.Amount)
	}
	if tp.LastTransaction != nil {
		t.Errorf("LastTransaction should be nil")
	}
	if !tp.Terminal.IsOnline {
		t.Errorf("IsOnline should be true")
	}
	if tp.Merchant.MCC != "5812" {
		t.Errorf("MCC: expected 5812, got %s", tp.Merchant.MCC)
	}
}
