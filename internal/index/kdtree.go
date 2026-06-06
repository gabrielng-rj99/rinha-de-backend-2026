package index

import (
	"sort"

	"rinha-backend-2026/internal/vector"
)

// LeafSize is the maximum number of vectors stored in a single KD-tree leaf.
const LeafSize = 32

// KDNode represents one node in a KD-tree.
type KDNode struct {
	IsLeaf   bool
	SplitDim int
	SplitVal float32
	Left     *KDNode
	Right    *KDNode
	BBoxMin  [14]float32
	BBoxMax  [14]float32

	// Leaf fields: index positions into the owning KDTree's Vectors slice.
	Indices []int
}

// KDTree is a KD-tree built over the vectors of a single partition.
type KDTree struct {
	Root    *KDNode
	BaseIdx int32          // global index of the first vector in this partition
	Vectors [][14]float32  // sub-slice referencing the mmap'd main array
	Labels  []uint8        // sub-slice referencing the mmap'd label array
}

// BuildKDTree constructs a new KD-tree from the supplied vectors and labels.
func BuildKDTree(vectors [][14]float32, labels []uint8, leafSize int) *KDTree {
	n := len(vectors)
	if n == 0 {
		return &KDTree{
			Vectors: vectors,
			Labels:  labels,
		}
	}

	// Index array that will be partitioned in-place during recursion.
	indices := make([]int, n)
	for i := 0; i < n; i++ {
		indices[i] = i
	}

	root := buildNode(indices, vectors, 0, leafSize)
	return &KDTree{
		Root:    root,
		Vectors: vectors,
		Labels:  labels,
	}
}

// buildNode recursively builds KD-tree nodes. The indices slice is sorted
// and split in-place; leaf nodes receive a copy of their portion.
func buildNode(
	indices []int, vectors [][14]float32, depth int, leafSize int,
) *KDNode {
	node := &KDNode{}

	// Compute bounding box.
	for d := 0; d < 14; d++ {
		node.BBoxMin[d] = vectors[indices[0]][d]
		node.BBoxMax[d] = vectors[indices[0]][d]
	}
	for _, idx := range indices[1:] {
		v := &vectors[idx]
		for d := 0; d < 14; d++ {
			if v[d] < node.BBoxMin[d] {
				node.BBoxMin[d] = v[d]
			}
			if v[d] > node.BBoxMax[d] {
				node.BBoxMax[d] = v[d]
			}
		}
	}

	// Small enough to be a leaf?
	if len(indices) <= leafSize {
		node.IsLeaf = true
		node.Indices = make([]int, len(indices))
		copy(node.Indices, indices)
		return node
	}

	// Pick the dimension with the widest spread.
	splitDim := 0
	widest := float32(0)
	for d := 0; d < 14; d++ {
		span := node.BBoxMax[d] - node.BBoxMin[d]
		if span > widest {
			widest = span
			splitDim = d
		}
	}

	// All vectors are identical — cannot split meaningfully.
	if widest == 0 {
		node.IsLeaf = true
		node.Indices = make([]int, len(indices))
		copy(node.Indices, indices)
		return node
	}

	// Sort by the split dimension and find the median.
	sort.Slice(indices, func(i, j int) bool {
		return vectors[indices[i]][splitDim] < vectors[indices[j]][splitDim]
	})

	mid := len(indices) / 2
	node.SplitDim = splitDim
	node.SplitVal = vectors[indices[mid]][splitDim]
	node.Left = buildNode(indices[:mid], vectors, depth+1, leafSize)
	node.Right = buildNode(indices[mid:], vectors, depth+1, leafSize)

	return node
}

// KDSearch returns the k nearest neighbors within this partition's KD-tree.
func (tree *KDTree) KDSearch(query *[14]float32, k int) []SearchResult {
	if tree == nil || tree.Root == nil {
		return nil
	}

	topK := NewTopK(k)

	// Explicit stack for DFS traversal.
	type frame struct {
		node *KDNode
	}
	stack := make([]frame, 0, 64)
	stack = append(stack, frame{node: tree.Root})

	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		// Bounding-box lower-bound pruning at the node level.
		if topK.Len() >= k {
			lb := bboxLowerBound(query, &f.node.BBoxMin, &f.node.BBoxMax)
			if lb >= topK.MaxDist() {
				continue
			}
		}

		if f.node.IsLeaf {
			// Compare against every vector in the leaf.
			for _, idx := range f.node.Indices {
				dist := vector.DistSquared(query, &tree.Vectors[idx])
				topK.Push(SearchResult{
					Index: tree.BaseIdx + int32(idx),
					Label: tree.Labels[idx],
					Dist:  dist,
				})
			}
		} else {
			// Push children so that the nearer one is visited first.
			diff := query[f.node.SplitDim] - f.node.SplitVal
			var first, second *KDNode
			if diff < 0 {
				first = f.node.Left
				second = f.node.Right
			} else {
				first = f.node.Right
				second = f.node.Left
			}
			if second != nil {
				stack = append(stack, frame{node: second})
			}
			if first != nil {
				stack = append(stack, frame{node: first})
			}
		}
	}

	return topK.Results()
}
