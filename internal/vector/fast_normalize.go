package vector

import (
	rinhttp "rinha-backend-2026/internal/http"
)

// FastNormalize parses a JSON payload using zero-allocation hand-rolled
// extractors instead of json.Unmarshal.  Produces the exact same [14]float32
// as Normalize — the computation is identical, only the JSON access differs.
func FastNormalize(payload []byte) ([14]float32, error) {
	// ---- Parse timestamps ----
	reqAt := rinhttp.ExtractString(payload, "transaction.requested_at")
	txTime, err := parseISO(reqAt)
	if err != nil {
		return [14]float32{}, err
	}

	// ---- Extract all fields ----
	amount := rinhttp.ExtractFloat(payload, "transaction.amount")
	installments := rinhttp.ExtractInt(payload, "transaction.installments")
	avgAmount := rinhttp.ExtractFloat(payload, "customer.avg_amount")
	txCount24h := rinhttp.ExtractInt(payload, "customer.tx_count_24h")
	kmFromHome := rinhttp.ExtractFloat(payload, "terminal.km_from_home")
	isOnline := rinhttp.ExtractBool(payload, "terminal.is_online")
	cardPresent := rinhttp.ExtractBool(payload, "terminal.card_present")
	merchantID := rinhttp.ExtractString(payload, "merchant.id")
	mcc := rinhttp.ExtractString(payload, "merchant.mcc")
	merchantAvg := rinhttp.ExtractFloat(payload, "merchant.avg_amount")
	knownMerchants := rinhttp.ExtractStringSlice(payload, "customer.known_merchants")
	lastTxNull := rinhttp.IsNull(payload, "last_transaction")

	// ---- Dimension 0: amount ----
	d0 := clamp(amount / maxAmount)

	// ---- Dimension 1: installments ----
	d1 := clamp(float64(installments) / maxInstallments)

	// ---- Dimension 2: amount vs average ----
	var d2 float64
	if avgAmount <= 0.0 {
		d2 = 1.0
	} else {
		d2 = clamp((amount / avgAmount) / amountVsAvgRatio)
	}

	// ---- Dimension 3: hour of day ----
	d3 := float64(txTime.Hour()) / 23.0

	// ---- Dimension 4: day of week ----
	d4 := float64(dowShift(txTime.Weekday())) / 6.0

	// ---- Dimensions 5, 6: last-transaction dependent ----
	var d5, d6 float64
	if !lastTxNull {
		lastTS := rinhttp.ExtractString(payload, "last_transaction.timestamp")
		lastTime, err := parseISO(lastTS)
		if err != nil {
			return [14]float32{}, err
		}
		minutesDiff := txTime.Sub(lastTime).Minutes()
		d5 = clamp(minutesDiff / maxMinutes)
		kmFromLast := rinhttp.ExtractFloat(payload, "last_transaction.km_from_current")
		d6 = clamp(kmFromLast / maxKm)
	} else {
		d5 = -1.0
		d6 = -1.0
	}

	// ---- Dimension 7: km from home ----
	d7 := clamp(kmFromHome / maxKm)

	// ---- Dimension 8: tx count 24h ----
	d8 := clamp(float64(txCount24h) / maxTxCount24h)

	// ---- Dimension 9: is online ----
	var d9 float64
	if isOnline {
		d9 = 1.0
	}

	// ---- Dimension 10: card present ----
	var d10 float64
	if cardPresent {
		d10 = 1.0
	}

	// ---- Dimension 11: unknown merchant ----
	var d11 float64
	if !inSlice(merchantID, knownMerchants) {
		d11 = 1.0
	}

	// ---- Dimension 12: MCC risk ----
	d12 := MccRisk(mcc)

	// ---- Dimension 13: merchant avg amount ----
	d13 := clamp(merchantAvg / maxMerchantAvgAmount)

	return [14]float32{
		float32(d0), float32(d1), float32(d2), float32(d3),
		float32(d4), float32(d5), float32(d6), float32(d7),
		float32(d8), float32(d9), float32(d10), float32(d11),
		float32(d12), float32(d13),
	}, nil
}
