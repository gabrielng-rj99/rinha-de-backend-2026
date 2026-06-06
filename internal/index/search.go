package index

import (
	"math"
	"sort"

	"rinha-backend-2026/internal/vector"
)

const (
	DefaultNPROBE   = 28
	LargeNPROBE     = 112
	EarlyExitThresh = 0
)

// SearchResult holds the result of a single nearest-neighbor lookup.
type SearchResult struct {
	Index int32
	Label uint8
	Dist  float64
}

// centroidDist pairs a centroid index with its distance from the query.
type centroidDist struct {
	idx  int
	dist float64
}

// ---------------------------------------------------------------------------
// TopK — max-heap that keeps the k smallest distances.
// ---------------------------------------------------------------------------

// TopK implements a bounded max-heap of SearchResults.
type TopK struct {
	entries []SearchResult
	k       int
}

// NewTopK creates a TopK that can hold up to k entries.
func NewTopK(k int) *TopK {
	return &TopK{
		entries: make([]SearchResult, 0, k),
		k:       k,
	}
}

// Push attempts to insert r into the heap. The heap retains at most k entries
// with the smallest Dist values.
func (h *TopK) Push(r SearchResult) {
	if len(h.entries) < h.k {
		h.entries = append(h.entries, r)
		h.bubbleUp(len(h.entries) - 1)
	} else if r.Dist < h.entries[0].Dist {
		h.entries[0] = r
		h.bubbleDown(0)
	}
}

// MaxDist returns the largest distance in the heap, or math.MaxFloat64 if
// the heap is not yet full.
func (h *TopK) MaxDist() float64 {
	if len(h.entries) < h.k {
		return math.MaxFloat64
	}
	return h.entries[0].Dist
}

// Len returns the current number of entries in the heap.
func (h *TopK) Len() int {
	return len(h.entries)
}

// Results returns the entries sorted by ascending distance.
func (h *TopK) Results() []SearchResult {
	sorted := make([]SearchResult, len(h.entries))
	copy(sorted, h.entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Dist < sorted[j].Dist
	})
	return sorted
}

func (h *TopK) bubbleUp(i int) {
	for i > 0 {
		p := (i - 1) / 2
		if h.entries[i].Dist <= h.entries[p].Dist {
			break
		}
		h.entries[i], h.entries[p] = h.entries[p], h.entries[i]
		i = p
	}
}

func (h *TopK) bubbleDown(i int) {
	n := len(h.entries)
	for {
		largest := i
		left := 2*i + 1
		right := 2*i + 2
		if left < n && h.entries[left].Dist > h.entries[largest].Dist {
			largest = left
		}
		if right < n && h.entries[right].Dist > h.entries[largest].Dist {
			largest = right
		}
		if largest == i {
			break
		}
		h.entries[i], h.entries[largest] = h.entries[largest], h.entries[i]
		i = largest
	}
}

// ---------------------------------------------------------------------------
// Two-stage IVF brute-force search
// ---------------------------------------------------------------------------

// Search runs the two-stage KNN search using brute-force scan of selected
// IVF partitions. No KD-tree — just linear scan with bounding-box pruning.
//
// Stage 1: probes DefaultNPROBE nearest centroids.
// Stage 2: if the fraud count is ambiguous (2 or 3), re-searches with
// LargeNPROBE centroids.
//
// Returns the number of fraudulent vectors (label==1) in the top 5 results.
func (idx *Index) Search(query *[14]float32) int {
	// Pre-compute distances from query to every centroid.
	n := int(idx.numCentroids)
	cd := make([]centroidDist, n)
	for i := 0; i < n; i++ {
		cd[i] = centroidDist{
			idx:  i,
			dist: vector.DistSquared(query, &idx.Centroids[i]),
		}
	}
	sort.Slice(cd, func(i, j int) bool {
		return cd[i].dist < cd[j].dist
	})

	fc := idx.searchNPROBE(query, cd, DefaultNPROBE)
	if fc == 2 || fc == 3 {
		fc = idx.searchNPROBE(query, cd, LargeNPROBE)
	}
	return fc
}

// searchNPROBE brute-force scans vectors in the nearest nprobe partitions.
func (idx *Index) searchNPROBE(
	query *[14]float32, cd []centroidDist, nprobe int,
) int {
	topK := NewTopK(5)

	limit := nprobe
	if limit > len(cd) {
		limit = len(cd)
	}

	for i := 0; i < limit; i++ {
		pi := cd[i].idx
		p := &idx.Partitions[pi]

		if p.Count == 0 {
			continue
		}

		// Bounding-box lower-bound pruning
		if topK.Len() >= 5 {
			lb := bboxLowerBound(query, &p.BBoxMin, &p.BBoxMax)
			if lb >= topK.MaxDist() {
				continue
			}
		}

		// Brute-force linear scan of all vectors in this partition
		start := int(p.Offset)
		end := start + int(p.Count)

		for j := start; j < end; j++ {
			d := vector.DistSquared(query, &idx.Vectors[j])
			if d < topK.MaxDist() {
				topK.Push(SearchResult{
					Index: int32(j),
					Label: idx.Labels[j],
					Dist:  d,
				})
			}
		}
	}

	res := topK.Results()
	fraudCount := 0
	for _, r := range res {
		if r.Label == 1 {
			fraudCount++
		}
	}
	return fraudCount
}

// bboxLowerBound returns the minimum possible squared Euclidean distance from
// query to any point inside the axis-aligned bounding box [bboxMin, bboxMax].
func bboxLowerBound(query *[14]float32, bboxMin, bboxMax *[14]float32) float64 {
	var sum float64
	for d := 0; d < 14; d++ {
		q := float64(query[d])
		min := float64(bboxMin[d])
		max := float64(bboxMax[d])
		if q < min {
			diff := min - q
			sum += diff * diff
		} else if q > max {
			diff := q - max
			sum += diff * diff
		}
	}
	return sum
}
