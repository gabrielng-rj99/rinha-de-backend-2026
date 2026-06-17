package vector

import (
	"encoding/json"
	"time"
)

// Scale factor used internally for MCC risk and other lookups.
const Scale = 10000.0

// Normalization constants.
const (
	maxAmount            = 10000.0
	maxInstallments      = 12.0
	amountVsAvgRatio     = 10.0
	maxMinutes           = 1440.0
	maxKm                = 1000.0
	maxTxCount24h        = 20.0
	maxMerchantAvgAmount = 10000.0
)

// ---------------------------------------------------------------------------
// JSON payload structs — must match the EXACT JSON structure
// ---------------------------------------------------------------------------

// TransactionPayload is the top-level normalization request.
type TransactionPayload struct {
	Transaction     TransactionData  `json:"transaction"`
	Customer        CustomerData     `json:"customer"`
	Terminal        TerminalData     `json:"terminal"`
	Merchant        MerchantData     `json:"merchant"`
	LastTransaction *LastTransaction `json:"last_transaction"`
}

// TransactionData holds the transaction-level fields.
type TransactionData struct {
	Amount       float64 `json:"amount"`
	Installments int     `json:"installments"`
	RequestedAt  string  `json:"requested_at"`
}

// LastTransaction holds optional previous-transaction details.
type LastTransaction struct {
	Timestamp     string  `json:"timestamp"`
	KmFromCurrent float64 `json:"km_from_current"`
}

// CustomerData holds customer-level fields.
type CustomerData struct {
	AvgAmount      float64  `json:"avg_amount"`
	TxCount24h     int      `json:"tx_count_24h"`
	KnownMerchants []string `json:"known_merchants"`
}

// TerminalData holds terminal-level fields.
type TerminalData struct {
	KmFromHome  float64 `json:"km_from_home"`
	IsOnline    bool    `json:"is_online"`
	CardPresent bool    `json:"card_present"`
}

// MerchantData holds merchant-level fields.
type MerchantData struct {
	ID        string  `json:"id"`
	MCC       string  `json:"mcc"`
	AvgAmount float64 `json:"avg_amount"`
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// clamp returns x clamped to [0.0, 1.0].
func clamp(x float64) float64 {
	if x < 0.0 {
		return 0.0
	}
	if x > 1.0 {
		return 1.0
	}
	return x
}

// parseISO parses an ISO 8601 timestamp string.
func parseISO(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, &time.ParseError{Layout: "ISO8601", Value: s}
}

// dowShift converts time.Weekday (Sun=0 … Sat=6) to Mon=0 … Sun=6.
func dowShift(w time.Weekday) int {
	return int((w + 6) % 7)
}

// inSlice reports whether s is present in ss.
func inSlice(s string, ss []string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Normalize
// ---------------------------------------------------------------------------

// Normalize parses a JSON payload and returns the normalized 14-dimensional
// vector as a [14]float32.
func Normalize(payload []byte) ([14]float32, error) {
	var req TransactionPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return [14]float32{}, err
	}

	// Parse timestamps (always in UTC).
	txTime, err := parseISO(req.Transaction.RequestedAt)
	if err != nil {
		return [14]float32{}, err
	}

	// ---------- dimension 0: amount ----------
	amount := clamp(req.Transaction.Amount / maxAmount)

	// ---------- dimension 1: installments ----------
	installments := clamp(float64(req.Transaction.Installments) / maxInstallments)

	// ---------- dimension 2: amount vs average ----------
	var amountVsAvg float64
	if req.Customer.AvgAmount <= 0.0 {
		amountVsAvg = 1.0
	} else {
		amountVsAvg = clamp((req.Transaction.Amount / req.Customer.AvgAmount) / amountVsAvgRatio)
	}

	// ---------- dimension 3: hour of day ----------
	hourOfDay := float64(txTime.Hour()) / 23.0

	// ---------- dimension 4: day of week ----------
	dayOfWeek := float64(dowShift(txTime.Weekday())) / 6.0

	// ---------- dimensions 5, 6: last-transaction dependent ----------
	var minutesSinceLast float64
	var kmFromLastTx float64

	if req.LastTransaction != nil {
		lastTime, err := parseISO(req.LastTransaction.Timestamp)
		if err != nil {
			return [14]float32{}, err
		}
		minutesDiff := txTime.Sub(lastTime).Minutes()
		minutesSinceLast = clamp(minutesDiff / maxMinutes)
		kmFromLastTx = clamp(req.LastTransaction.KmFromCurrent / maxKm)
	} else {
		minutesSinceLast = -1.0 // sentinel
		kmFromLastTx = -1.0     // sentinel
	}

	// ---------- dimension 7: km from home ----------
	kmFromHome := clamp(req.Terminal.KmFromHome / maxKm)

	// ---------- dimension 8: tx count 24h ----------
	txCount24h := clamp(float64(req.Customer.TxCount24h) / maxTxCount24h)

	// ---------- dimension 9: is online ----------
	var isOnline float64
	if req.Terminal.IsOnline {
		isOnline = 1.0
	}

	// ---------- dimension 10: card present ----------
	var cardPresent float64
	if req.Terminal.CardPresent {
		cardPresent = 1.0
	}

	// ---------- dimension 11: unknown merchant ----------
	var unknownMerchant float64
	if !inSlice(req.Merchant.ID, req.Customer.KnownMerchants) {
		unknownMerchant = 1.0
	}

	// ---------- dimension 12: MCC risk ----------
	mccRisk := MccRisk(req.Merchant.MCC)

	// ---------- dimension 13: merchant avg amount ----------
	merchantAvgAmount := clamp(req.Merchant.AvgAmount / maxMerchantAvgAmount)

	// ---------- return as float32 ----------
	return [14]float32{
		float32(amount),
		float32(installments),
		float32(amountVsAvg),
		float32(hourOfDay),
		float32(dayOfWeek),
		float32(minutesSinceLast),
		float32(kmFromLastTx),
		float32(kmFromHome),
		float32(txCount24h),
		float32(isOnline),
		float32(cardPresent),
		float32(unknownMerchant),
		float32(mccRisk),
		float32(merchantAvgAmount),
	}, nil
}
