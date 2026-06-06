package index

import (
	"math"
	"math/rand/v2"
	"runtime"
	"sync"

	"rinha-backend-2026/internal/vector"
)

// KMeans performs k-means clustering on the given vectors using k-means++
// initialization and returns the final centroids.
// Parallelized across CPU cores for the assignment step.
func KMeans(vectors [][14]float32, k int, iterations int) [][14]float32 {
	n := len(vectors)
	if n == 0 || k <= 0 {
		return nil
	}
	if k > n {
		k = n
	}

	rng := rand.New(rand.NewPCG(42, 0))
	numWorkers := runtime.NumCPU()
	if numWorkers > 16 {
		numWorkers = 16
	}

	centroids := make([][14]float32, k)

	// ---- k-means++ initialization ----
	// First centroid: random
	centroids[0] = vectors[rng.IntN(n)]

	// For k-means++, sample a subset to speed up initialization
	// For large n, computing distances over all vectors for each of 2048 centroids is too slow
	const initSampleSize = 100000 // sample for distance estimation

	for c := 1; c < k; c++ {
		// Compute distances from sample to nearest existing centroid
		sampleSize := initSampleSize
		if sampleSize > n {
			sampleSize = n
		}

		type distIdx struct {
			dist float64
			idx  int
		}
		dists := make([]distIdx, sampleSize)
		var total float64

		for si := 0; si < sampleSize; si++ {
			i := si * n / sampleSize // evenly spaced sampling
			v := vectors[i]
			minDist := math.MaxFloat64
			for j := 0; j < c; j++ {
				d := vector.DistSquared(&v, &centroids[j])
				if d < minDist {
					minDist = d
				}
			}
			dists[si] = distIdx{minDist, i}
			total += minDist
		}

		// Weighted random selection
		threshold := rng.Float64() * total
		var cumulative float64
		chosen := dists[0].idx
		for _, di := range dists {
			cumulative += di.dist
			if cumulative >= threshold {
				chosen = di.idx
				break
			}
		}
		centroids[c] = vectors[chosen]
	}

	// ---- Lloyd iterations ----
	labels := make([]int, n)
	chunkSize := (n + numWorkers - 1) / numWorkers

	for iter := 0; iter < iterations; iter++ {
		// Parallel assignment step
		var changed atomicBool

		var wg sync.WaitGroup
		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			start := w * chunkSize
			end := start + chunkSize
			if end > n {
				end = n
			}
			go func(s, e int) {
				defer wg.Done()
				for i := s; i < e; i++ {
					v := &vectors[i]
					minDist := math.MaxFloat64
					best := 0
					for j := range centroids {
						d := vector.DistSquared(v, &centroids[j])
						if d < minDist {
							minDist = d
							best = j
						}
					}
					if labels[i] != best {
						labels[i] = best
						changed.set()
					}
				}
			}(start, end)
		}
		wg.Wait()

		if !changed.get() {
			break
		}

		// Update step: recompute centroids
		counts := make([]int, k)
		sums := make([][14]float64, k)
		for i, v := range vectors {
			l := labels[i]
			counts[l]++
			for d := 0; d < 14; d++ {
				sums[l][d] += float64(v[d])
			}
		}
		for j := 0; j < k; j++ {
			if counts[j] > 0 {
				for d := 0; d < 14; d++ {
					centroids[j][d] = float32(sums[j][d] / float64(counts[j]))
				}
			}
		}
	}

	return centroids
}

// AssignPartitions assigns each vector to the nearest centroid and returns
// the partition (centroid) index for each vector. Parallelized.
func AssignPartitions(vectors [][14]float32, centroids [][14]float32) []int {
	n := len(vectors)
	labels := make([]int, n)
	numWorkers := runtime.NumCPU()
	if numWorkers > 16 {
		numWorkers = 16
	}
	chunkSize := (n + numWorkers - 1) / numWorkers

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		start := w * chunkSize
		end := start + chunkSize
		if end > n {
			end = n
		}
		go func(s, e int) {
			defer wg.Done()
			for i := s; i < e; i++ {
				v := &vectors[i]
				minDist := math.MaxFloat64
				best := 0
				for j := range centroids {
					d := vector.DistSquared(v, &centroids[j])
					if d < minDist {
						minDist = d
						best = j
					}
				}
				labels[i] = best
			}
		}(start, end)
	}
	wg.Wait()

	return labels
}

// atomicBool is a simple atomic boolean flag.
type atomicBool struct {
	v uint32
}

func (b *atomicBool) set() {
	// Simple non-atomic set - we only need visibility after WaitGroup
	b.v = 1
}

func (b *atomicBool) get() bool {
	return b.v != 0
}
