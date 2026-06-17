package vector

// DistSquared computes the squared Euclidean distance between two [14]float32
// vectors using a float64 accumulator (matching C's double precision).
// The function is fully unrolled for 14 dimensions.
func DistSquared(a, b *[14]float32) float64 {
	diff := float64(a[0]) - float64(b[0])
	sum := diff * diff
	diff = float64(a[1]) - float64(b[1])
	sum += diff * diff
	diff = float64(a[2]) - float64(b[2])
	sum += diff * diff
	diff = float64(a[3]) - float64(b[3])
	sum += diff * diff
	diff = float64(a[4]) - float64(b[4])
	sum += diff * diff
	diff = float64(a[5]) - float64(b[5])
	sum += diff * diff
	diff = float64(a[6]) - float64(b[6])
	sum += diff * diff
	diff = float64(a[7]) - float64(b[7])
	sum += diff * diff
	diff = float64(a[8]) - float64(b[8])
	sum += diff * diff
	diff = float64(a[9]) - float64(b[9])
	sum += diff * diff
	diff = float64(a[10]) - float64(b[10])
	sum += diff * diff
	diff = float64(a[11]) - float64(b[11])
	sum += diff * diff
	diff = float64(a[12]) - float64(b[12])
	sum += diff * diff
	diff = float64(a[13]) - float64(b[13])
	sum += diff * diff
	return sum
}
