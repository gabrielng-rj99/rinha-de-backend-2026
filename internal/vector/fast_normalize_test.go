package vector

import (
	"testing"
)

func TestFastNormalizeMatchesNormalize(t *testing.T) {
	payloads := []string{
		`{
			"transaction": {"amount": 5000.00, "installments": 3, "requested_at": "2026-01-15T14:30:00Z"},
			"customer": {"avg_amount": 200.00, "tx_count_24h": 5, "known_merchants": ["m001", "m002"]},
			"terminal": {"km_from_home": 15.0, "is_online": true, "card_present": false},
			"merchant": {"id": "m001", "mcc": "5411", "avg_amount": 300.00},
			"last_transaction": {"timestamp": "2026-01-15T14:00:00Z", "km_from_current": 50.0}
		}`,
		`{
			"transaction": {"amount": 100.00, "installments": 1, "requested_at": "2026-06-01T10:00:00Z"},
			"customer": {"avg_amount": 50.00, "tx_count_24h": 0, "known_merchants": ["m001"]},
			"terminal": {"km_from_home": 0.0, "is_online": false, "card_present": true},
			"merchant": {"id": "unknown_99", "mcc": "9999", "avg_amount": 0.0},
			"last_transaction": null
		}`,
		`{
			"transaction": {"amount": 99999.99, "installments": 99, "requested_at": "2026-01-15T14:30:00Z"},
			"customer": {"avg_amount": 0.0, "tx_count_24h": 999, "known_merchants": []},
			"terminal": {"km_from_home": 99999.0, "is_online": false, "card_present": false},
			"merchant": {"id": "x", "mcc": "5411", "avg_amount": 99999.0},
			"last_transaction": null
		}`,
	}

	for i, p := range payloads {
		slow, err := Normalize([]byte(p))
		if err != nil {
			t.Fatalf("Normalize failed on payload %d: %v", i, err)
		}
		fast, err := FastNormalize([]byte(p))
		if err != nil {
			t.Fatalf("FastNormalize failed on payload %d: %v", i, err)
		}
		for d := 0; d < 14; d++ {
			if slow[d] != fast[d] {
				t.Errorf("payload %d dim %d: Normalize=%f FastNormalize=%f", i, d, slow[d], fast[d])
			}
		}
	}
}

func BenchmarkNormalize(b *testing.B) {
	payload := []byte(`{"transaction":{"amount":5000,"installments":3,"requested_at":"2026-01-15T14:30:00Z"},"customer":{"avg_amount":200,"tx_count_24h":5,"known_merchants":["m001","m002"]},"terminal":{"km_from_home":15,"is_online":true,"card_present":false},"merchant":{"id":"m001","mcc":"5411","avg_amount":300},"last_transaction":{"timestamp":"2026-01-15T14:00:00Z","km_from_current":50}}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Normalize(payload)
	}
}

func BenchmarkFastNormalize(b *testing.B) {
	payload := []byte(`{"transaction":{"amount":5000,"installments":3,"requested_at":"2026-01-15T14:30:00Z"},"customer":{"avg_amount":200,"tx_count_24h":5,"known_merchants":["m001","m002"]},"terminal":{"km_from_home":15,"is_online":true,"card_present":false},"merchant":{"id":"m001","mcc":"5411","avg_amount":300},"last_transaction":{"timestamp":"2026-01-15T14:00:00Z","km_from_current":50}}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FastNormalize(payload)
	}
}
