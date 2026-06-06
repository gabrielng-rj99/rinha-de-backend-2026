package http

import (
	"testing"
)

// ---------------------------------------------------------------------------
// parser.go tests
// ---------------------------------------------------------------------------

func TestParseRequest_GET_ready(t *testing.T) {
	raw := []byte("GET /ready HTTP/1.1\r\nHost: localhost\r\nUser-Agent: test\r\n\r\n")
	method, path, body, n := ParseRequest(raw)
	if method != "GET" {
		t.Fatalf("expected GET, got %q", method)
	}
	if path != "/ready" {
		t.Fatalf("expected /ready, got %q", path)
	}
	if body != nil {
		t.Fatalf("expected nil body, got %q", body)
	}
	if n != len(raw) {
		t.Fatalf("expected bytesRead %d, got %d", len(raw), n)
	}
}

func TestParseRequest_POST_fraudscore(t *testing.T) {
	jsonBody := `{"amount":100.50,"installments":3}`
	raw := []byte("POST /fraud-score HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\nContent-Length: " + itoa(len(jsonBody)) + "\r\n\r\n" + jsonBody)
	method, path, body, n := ParseRequest(raw)
	if method != "POST" {
		t.Fatalf("expected POST, got %q", method)
	}
	if path != "/fraud-score" {
		t.Fatalf("expected /fraud-score, got %q", path)
	}
	if string(body) != jsonBody {
		t.Fatalf("expected body %q, got %q", jsonBody, string(body))
	}
	if n != len(raw) {
		t.Fatalf("expected bytesRead %d, got %d", len(raw), n)
	}
}

func TestParseRequest_KeepAlive(t *testing.T) {
	// Two requests concatenated
	body1 := `{"a":1}`
	req1 := "POST /fraud-score HTTP/1.1\r\nHost: a\r\nContent-Length: " + itoa(len(body1)) + "\r\n\r\n" + body1
	req2 := "GET /ready HTTP/1.1\r\nHost: b\r\n\r\n"
	raw := []byte(req1 + req2)

	method, path, body, n1 := ParseRequest(raw)
	if method != "POST" || path != "/fraud-score" || string(body) != body1 {
		t.Fatalf("first request failed: method=%q path=%q body=%q", method, path, string(body))
	}
	// Second request starts at n1
	remaining := raw[n1:]
	method2, path2, body2b, _ := ParseRequest(remaining)
	if method2 != "GET" || path2 != "/ready" || body2b != nil {
		t.Fatalf("second request failed: method=%q path=%q body=%q", method2, path2, string(body2b))
	}
}

func TestParseRequest_NoBody(t *testing.T) {
	raw := []byte("POST /fraud-score HTTP/1.1\r\nHost: localhost\r\nContent-Length: 0\r\n\r\n")
	_, _, body, n := ParseRequest(raw)
	if body != nil {
		t.Fatalf("expected nil body for zero Content-Length, got %q", body)
	}
	if n != len(raw) {
		t.Fatalf("expected bytesRead %d, got %d", len(raw), n)
	}
}

func TestParseRequest_Truncated(t *testing.T) {
	raw := []byte("GET /ready HTTP/1.1\r\nHo")
	_, _, _, n := ParseRequest(raw)
	if n != 0 {
		t.Fatalf("expected 0 for truncated input, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// json.go tests
// ---------------------------------------------------------------------------

var testJSON = []byte(`{
	"amount": 100.50,
	"installments": 3,
	"requested_at": "2026-06-05T12:00:00Z",
	"customer": {
		"avg_amount": 50.0,
		"tx_count_24h": 10,
		"known_merchants": ["merchant_a", "merchant_b", "merchant_c"]
	},
	"merchant": {
		"id": "m001",
		"mcc": "5812",
		"is_online": true,
		"card_present": false,
		"km_from_home": 5.5
	},
	"last_transaction": {
		"timestamp": "2026-06-05T10:00:00Z",
		"km_from_current": 1.25
	}
}`)

func TestExtractFloat(t *testing.T) {
	v := ExtractFloat(testJSON, "amount")
	if v != 100.50 {
		t.Fatalf("expected 100.50, got %f", v)
	}
}

func TestExtractFloat_Nested(t *testing.T) {
	v := ExtractFloat(testJSON, "customer.avg_amount")
	if v != 50.0 {
		t.Fatalf("expected 50.0, got %f", v)
	}
	km := ExtractFloat(testJSON, "merchant.km_from_home")
	if km != 5.5 {
		t.Fatalf("expected 5.5, got %f", km)
	}
	km2 := ExtractFloat(testJSON, "last_transaction.km_from_current")
	if km2 != 1.25 {
		t.Fatalf("expected 1.25, got %f", km2)
	}
}

func TestExtractInt(t *testing.T) {
	v := ExtractInt(testJSON, "installments")
	if v != 3 {
		t.Fatalf("expected 3, got %d", v)
	}
}

func TestExtractInt_Nested(t *testing.T) {
	v := ExtractInt(testJSON, "customer.tx_count_24h")
	if v != 10 {
		t.Fatalf("expected 10, got %d", v)
	}
}

func TestExtractString(t *testing.T) {
	v := ExtractString(testJSON, "requested_at")
	if v != "2026-06-05T12:00:00Z" {
		t.Fatalf("expected ISO timestamp, got %q", v)
	}
}

func TestExtractString_Nested(t *testing.T) {
	v := ExtractString(testJSON, "merchant.id")
	if v != "m001" {
		t.Fatalf("expected 'm001', got %q", v)
	}
	v2 := ExtractString(testJSON, "last_transaction.timestamp")
	if v2 != "2026-06-05T10:00:00Z" {
		t.Fatalf("expected last_transaction timestamp, got %q", v2)
	}
}

func TestExtractBool(t *testing.T) {
	v := ExtractBool(testJSON, "merchant.is_online")
	if v != true {
		t.Fatalf("expected true, got %v", v)
	}
	v2 := ExtractBool(testJSON, "merchant.card_present")
	if v2 != false {
		t.Fatalf("expected false, got %v", v2)
	}
}

func TestExtractStringSlice(t *testing.T) {
	v := ExtractStringSlice(testJSON, "customer.known_merchants")
	if len(v) != 3 {
		t.Fatalf("expected 3 merchants, got %d: %v", len(v), v)
	}
	if v[0] != "merchant_a" || v[1] != "merchant_b" || v[2] != "merchant_c" {
		t.Fatalf("unexpected merchants: %v", v)
	}
}

func TestExtractMCC(t *testing.T) {
	v := ExtractString(testJSON, "merchant.mcc")
	if v != "5812" {
		t.Fatalf("expected '5812', got %q", v)
	}
}

func TestIsNull_Object(t *testing.T) {
	// last_transaction is an object, not null
	v := IsNull(testJSON, "last_transaction")
	if v != false {
		t.Fatalf("expected false (last_transaction is an object), got true")
	}
}

func TestIsNull_Null(t *testing.T) {
	nullJSON := []byte(`{"last_transaction": null, "amount": 1}`)
	v := IsNull(nullJSON, "last_transaction")
	if v != true {
		t.Fatalf("expected true (last_transaction is null), got false")
	}
}

func TestExtractFloat_MissingKey(t *testing.T) {
	v := ExtractFloat(testJSON, "nonexistent")
	if v != 0 {
		t.Fatalf("expected 0 for missing key, got %f", v)
	}
}

func TestExtractInt_MissingKey(t *testing.T) {
	v := ExtractInt(testJSON, "customer.nonexistent")
	if v != 0 {
		t.Fatalf("expected 0 for missing nested key, got %d", v)
	}
}

// ---------------------------------------------------------------------------
// response.go tests
// ---------------------------------------------------------------------------

func TestFraudResponses_Count(t *testing.T) {
	if len(FraudResponses) != 6 {
		t.Fatalf("expected 6 responses, got %d", len(FraudResponses))
	}
}

func TestFraudResponses_Content(t *testing.T) {
	expectedBodies := []string{
		`{"approved":true,"fraud_score":0.0}`,
		`{"approved":true,"fraud_score":0.2}`,
		`{"approved":true,"fraud_score":0.4}`,
		`{"approved":false,"fraud_score":0.6}`,
		`{"approved":false,"fraud_score":0.8}`,
		`{"approved":false,"fraud_score":1.0}`,
	}
	for i := 0; i < 6; i++ {
		resp := FraudResponses[i]
		// Parse the response to verify body
		_, _, body, _ := ParseRequest(resp)
		if string(body) != expectedBodies[i] {
			t.Fatalf("response[%d]: expected body %q, got %q", i, expectedBodies[i], string(body))
		}
		// Verify status line
		if len(resp) < 17 || string(resp[:15]) != "HTTP/1.1 200 OK" {
			t.Fatalf("response[%d]: bad status line", i)
		}
		// Verify Content-Type
		if !containsBytes(resp, "Content-Type: application/json") {
			t.Fatalf("response[%d]: missing Content-Type header", i)
		}
		// Verify Connection: keep-alive
		if !containsBytes(resp, "Connection: keep-alive") {
			t.Fatalf("response[%d]: missing Connection: keep-alive", i)
		}
		// Verify correct Content-Length
		cl := len(expectedBodies[i])
		clStr := itoa(cl)
		if !containsBytes(resp, "Content-Length: "+clStr) {
			t.Fatalf("response[%d]: expected Content-Length: %s", i, clStr)
		}
	}
}

func TestReadyResponse(t *testing.T) {
	resp := ReadyResponse
	// Verify status
	if len(resp) < 17 || string(resp[:15]) != "HTTP/1.1 200 OK" {
		t.Fatalf("ready response: bad status line")
	}
	// Verify zero body
	if !containsBytes(resp, "Content-Length: 0") {
		t.Fatalf("ready response: missing Content-Length: 0")
	}
	if !containsBytes(resp, "Connection: keep-alive") {
		t.Fatalf("ready response: missing Connection: keep-alive")
	}
	// Parse and check no body
	_, _, body, _ := ParseRequest(resp)
	if body != nil {
		t.Fatalf("ready response: expected nil body, got %q", body)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func containsBytes(b []byte, s string) bool {
	if len(s) > len(b) {
		return false
	}
	for i := 0; i <= len(b)-len(s); i++ {
		match := true
		for j := 0; j < len(s); j++ {
			if b[i+j] != s[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
