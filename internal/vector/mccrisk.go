package vector

// mccRiskTable maps merchant category codes to risk scores (0.0-1.0).
var mccRiskTable = map[string]float64{
	"5411": 0.15, // Grocery stores, supermarkets
	"5812": 0.30, // Eating places, restaurants
	"5912": 0.20, // Drug stores, pharmacies
	"5944": 0.45, // Jewelry, watch, silverware stores
	"7801": 0.80, // Government licensed on-line casinos
	"7802": 0.75, // Government licensed horse/dog racing
	"7995": 0.85, // Betting, lottery, sweepstakes
	"4511": 0.35, // Airlines
	"5311": 0.25, // Department stores
	"5999": 0.50, // Miscellaneous specialty retail
}

// MccRisk returns the risk score for a given MCC code.
// Returns 0.5 if the MCC is not found in the table.
func MccRisk(mcc string) float64 {
	if risk, ok := mccRiskTable[mcc]; ok {
		return risk
	}
	return 0.5
}
