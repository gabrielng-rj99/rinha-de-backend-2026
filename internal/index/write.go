package index

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

// WriteIndex writes the binary index file at path.
//
// Binary layout (must match LoadIndex / parseArrays):
//
//	Header (64 bytes):
//	  [0:8]   magic       "RINHAIDX"
//	  [8:12]  version      uint32 LE
//	  [12:16] num_vectors  uint32 LE
//	  [16:20] num_centroids uint32 LE
//	  [20:22] dim          uint16 LE  (=14)
//	  [22:24] scale        uint16 LE  (=0 for float32)
//	  [24:64] padding      zeros
//
//	Centroids:  [num_centroids][14]float32   (each dim = 4 bytes, LE float32)
//	Partitions: [num_centroids]PartitionEntry
//	Vectors:    [num_vectors][14]float32     (sorted by partition)
//	Labels:     [num_vectors]uint8
func WriteIndex(
	path string,
	centroids [][14]float32,
	partitions []PartitionEntry,
	vectors [][14]float32,
	labels []uint8,
) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	nv := uint32(len(vectors))
	nc := uint32(len(centroids))

	// ── Header ──────────────────────────────────────────────────────────
	hdr := make([]byte, HeaderSize)
	copy(hdr[0:8], Magic)
	binary.LittleEndian.PutUint32(hdr[8:12], Version)
	binary.LittleEndian.PutUint32(hdr[12:16], nv)
	binary.LittleEndian.PutUint32(hdr[16:20], nc)
	binary.LittleEndian.PutUint16(hdr[20:22], Dim)
	binary.LittleEndian.PutUint16(hdr[22:24], Scale)
	// hdr[24:64] is already zeroed

	if _, err := f.Write(hdr); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// ── Centroids ───────────────────────────────────────────────────────
	// Write each [14]float32 in little-endian order.
	cbuf := make([]byte, nc*Dim*4)
	for i, c := range centroids {
		base := i * Dim * 4
		for d := 0; d < Dim; d++ {
			binary.LittleEndian.PutUint32(cbuf[base+d*4:], math.Float32bits(c[d]))
		}
	}
	if _, err := f.Write(cbuf); err != nil {
		return fmt.Errorf("write centroids: %w", err)
	}

	// ── Partition entries ──────────────────────────────────────────────
	// PartitionEntry layout: Offset(uint64) + Count(uint32) + CentroidIdx(uint32)
	//   + BBoxMin([14]float32) + BBoxMax([14]float32) = 8+4+4+56+56 = 128 bytes.
	for _, p := range partitions {
		buf := make([]byte, unsafeSizeofPartitionEntry)
		binary.LittleEndian.PutUint64(buf[0:8], p.Offset)
		binary.LittleEndian.PutUint32(buf[8:12], p.Count)
		binary.LittleEndian.PutUint32(buf[12:16], p.CentroidIdx)
		for d := 0; d < Dim; d++ {
			binary.LittleEndian.PutUint32(buf[16+d*4:], math.Float32bits(p.BBoxMin[d]))
		}
		for d := 0; d < Dim; d++ {
			binary.LittleEndian.PutUint32(buf[16+Dim*4+d*4:], math.Float32bits(p.BBoxMax[d]))
		}
		if _, err := f.Write(buf); err != nil {
			return fmt.Errorf("write partition entry %d: %w", p.CentroidIdx, err)
		}
	}

	// ── Vectors ─────────────────────────────────────────────────────────
	vbuf := make([]byte, nv*Dim*4)
	for i, v := range vectors {
		base := i * Dim * 4
		for d := 0; d < Dim; d++ {
			binary.LittleEndian.PutUint32(vbuf[base+d*4:], math.Float32bits(v[d]))
		}
	}
	if _, err := f.Write(vbuf); err != nil {
		return fmt.Errorf("write vectors: %w", err)
	}

	// ── Labels ──────────────────────────────────────────────────────────
	if _, err := f.Write(labels); err != nil {
		return fmt.Errorf("write labels: %w", err)
	}

	return nil
}

// unsafeSizeofPartitionEntry is a compile-time constant for the serialized
// size of a PartitionEntry (8+4+4+56+56 = 128).
const unsafeSizeofPartitionEntry = 128
