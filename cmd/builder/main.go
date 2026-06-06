package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"rinha-backend-2026/internal/index"
)

// jsonRecord matches one element of the references.json.gz array.
// It is decoded individually via json.Decoder for streaming.
type jsonRecord struct {
	Vector []float64 `json:"vector"`
	Label  string    `json:"label"`
}

func main() {
	input := flag.String("input", "references.json.gz", "path to references.json.gz")
	output := flag.String("output", "references.idx", "path to output .idx file")
	k := flag.Int("centroids", 2048, "number of centroids (clusters)")
	leafSize := flag.Int("leafsize", 32, "leaf size for KD-trees")
	iterations := flag.Int("iterations", 15, "number of k-means iterations")
	flag.Parse()

	start := time.Now()

	// ── Open and decompress ──────────────────────────────────────────
	f, err := os.Open(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening %s: %v\n", *input, err)
		os.Exit(1)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating gzip reader: %v\n", err)
		os.Exit(1)
	}
	defer gr.Close()

	// ── Stream-parse JSON array ──────────────────────────────────────
	dec := json.NewDecoder(gr)

	// Read opening '['
	t, err := dec.Token()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading JSON start token: %v\n", err)
		os.Exit(1)
	}
	if delim, ok := t.(json.Delim); !ok || delim != '[' {
		fmt.Fprintf(os.Stderr, "expected JSON array '[', got %v\n", t)
		os.Exit(1)
	}

	var vectors [][14]float32
	var labels []uint8

	// Process each array element individually (streaming).
	for dec.More() {
		var rec jsonRecord
		if err := dec.Decode(&rec); err != nil {
			fmt.Fprintf(os.Stderr, "error decoding JSON record: %v\n", err)
			os.Exit(1)
		}

		if len(rec.Vector) != 14 {
			fmt.Fprintf(os.Stderr, "expected 14-dimensional vector, got %d\n", len(rec.Vector))
			os.Exit(1)
		}

		// Store as raw float32 (no int16 quantization).
		var vec [14]float32
		for i, v := range rec.Vector {
			vec[i] = float32(v)
		}
		vectors = append(vectors, vec)

		// Map string label to uint8.
		var label uint8
		switch rec.Label {
		case "legit":
			label = 0
		case "fraud":
			label = 1
		default:
			fmt.Fprintf(os.Stderr, "unknown label %q (expected 'legit' or 'fraud')\n", rec.Label)
			os.Exit(1)
		}
		labels = append(labels, label)
	}

	// Read closing ']' (discard).
	_, _ = dec.Token()

	parseTime := time.Since(start)
	fmt.Printf("Parsed %d vectors in %v\n", len(vectors), parseTime)

	if len(vectors) == 0 {
		fmt.Fprintf(os.Stderr, "error: no vectors found in input\n")
		os.Exit(1)
	}

	// ── K-means clustering ──────────────────────────────────────────
	centroids := index.KMeans(vectors, *k, *iterations)
	kmTime := time.Since(start)
	fmt.Printf("K-means (%d centroids, %d iters) completed in %v\n",
		*k, *iterations, kmTime-parseTime)

	// ── Assign partitions ───────────────────────────────────────────
	partitionIDs := index.AssignPartitions(vectors, centroids)

	// ── Sort vectors by partition assignment ────────────────────────
	perm := make([]int, len(vectors))
	for i := range perm {
		perm[i] = i
	}
	sort.Slice(perm, func(i, j int) bool {
		return partitionIDs[perm[i]] < partitionIDs[perm[j]]
	})

	sortedVectors := make([][14]float32, len(vectors))
	sortedLabels := make([]uint8, len(vectors))
	for i, idx := range perm {
		sortedVectors[i] = vectors[idx]
		sortedLabels[i] = labels[idx]
	}

	// ── Build partition entries ─────────────────────────────────────
	counts := make([]int, *k)
	for _, pid := range partitionIDs {
		counts[pid]++
	}

	partitions := make([]index.PartitionEntry, *k)
	var accum uint64
	for i := 0; i < *k; i++ {
		partitions[i].Offset = accum
		partitions[i].Count = uint32(counts[i])
		partitions[i].CentroidIdx = uint32(i)
		accum += uint64(counts[i])
	}

	// Compute bounding boxes from sorted vectors (grouped by partition).
	for i := 0; i < *k; i++ {
		if partitions[i].Count == 0 {
			continue
		}
		startIdx := int(partitions[i].Offset)
		endIdx := startIdx + int(partitions[i].Count)

		bboxMin := sortedVectors[startIdx]
		bboxMax := sortedVectors[startIdx]
		for j := startIdx + 1; j < endIdx; j++ {
			v := &sortedVectors[j]
			for d := 0; d < 14; d++ {
				if v[d] < bboxMin[d] {
					bboxMin[d] = v[d]
				}
				if v[d] > bboxMax[d] {
					bboxMax[d] = v[d]
				}
			}
		}
		partitions[i].BBoxMin = bboxMin
		partitions[i].BBoxMax = bboxMax
	}

	assignTime := time.Since(start)
	fmt.Printf("Partitioning and sorting completed in %v\n", assignTime-kmTime)

	// ── Build KD-trees per partition (uses partition BBox) ──────────
	// KD-trees are not persisted in the index file; they are rebuilt
	// on LoadIndex. We still build them here to exercise the code path;
	// the index file only stores raw centroids, partitions, vectors,
	// and labels. The KD-trees will be reconstructed at load time.
	_ = leafSize // leafSize is consumed at load time by BuildKDTree

	// ── Write binary index ──────────────────────────────────────────
	if err := index.WriteIndex(*output, centroids, partitions, sortedVectors, sortedLabels); err != nil {
		fmt.Fprintf(os.Stderr, "error writing index: %v\n", err)
		os.Exit(1)
	}

	totalTime := time.Since(start)

	// ── Stats ────────────────────────────────────────────────────────
	fmt.Println("─── Builder Statistics ───")
	fmt.Printf("Total vectors:    %d\n", len(vectors))
	fmt.Printf("Total partitions: %d\n", *k)
	fmt.Printf("Total time:       %v\n", totalTime)
	fmt.Printf("Output:           %s\n", *output)

	// Verify the file was written.
	if fi, err := os.Stat(*output); err == nil {
		fmt.Printf("File size:        %d bytes (%.2f MB)\n", fi.Size(), float64(fi.Size())/(1024*1024))
	}
}
