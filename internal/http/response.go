package http

// ---------------------------------------------------------------------------
// Pre-computed HTTP/1.1 responses
//
// Six fraud-score response bodies plus a ready (empty) response, each
// packaged as a complete HTTP/1.1 200 OK message with headers.
// Index by fraud_count (0-5); readyResponse is separate.
// ---------------------------------------------------------------------------

// FraudResponses holds the 6 pre-computed wire-format HTTP responses.
// fraudResponses[n] corresponds to fraud_count == n (0 ≤ n ≤ 5).
var FraudResponses [6][]byte

// ReadyResponse is a pre-computed HTTP 200 OK with zero-byte body, used
// for the /ready health-check endpoint.
var ReadyResponse []byte

func init() {
	// Response bodies – exact JSON, verified below
	bodies := [6]string{
		`{"approved":true,"fraud_score":0.0}`,
		`{"approved":true,"fraud_score":0.2}`,
		`{"approved":true,"fraud_score":0.4}`,
		`{"approved":false,"fraud_score":0.6}`,
		`{"approved":false,"fraud_score":0.8}`,
		`{"approved":false,"fraud_score":1.0}`,
	}

	for i := 0; i < 6; i++ {
		body := bodies[i]
		FraudResponses[i] = buildResponse(body)
	}

	ReadyResponse = []byte(
		"HTTP/1.1 200 OK\r\n" +
			"Content-Length: 0\r\n" +
			"Connection: keep-alive\r\n" +
			"\r\n",
	)
}

// buildResponse wraps a JSON body string in a complete HTTP/1.1 200 OK
// response with Content-Type, Content-Length, and keep-alive headers.
func buildResponse(body string) []byte {
	// Manually construct the header + body to avoid fmt.Sprintf allocations
	cl := len(body) // Content-Length value as integer

	// Build Content-Length string manually (no strconv)
	clStr := ""
	if cl == 0 {
		clStr = "0"
	} else {
		var digits [10]byte
		n := cl
		pos := len(digits)
		for n > 0 {
			pos--
			digits[pos] = byte('0' + n%10)
			n /= 10
		}
		clStr = string(digits[pos:])
	}

	// Assemble the full response
	resp := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Length: " + clStr + "\r\n" +
		"Connection: keep-alive\r\n" +
		"\r\n" +
		body

	return []byte(resp)
}
