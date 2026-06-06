package index

import (
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	Magic      = "RINHAIDX"
	Version    = 1
	NumVectors = 3000000
	Dim        = 14
	Scale      = 0 // 0 indicates raw float32 elements (no int16 quantization)
	HeaderSize = 64
)

// PartitionEntry describes one IVF partition in the index file.
type PartitionEntry struct {
	Offset      uint64
	Count       uint32
	CentroidIdx uint32
	BBoxMin     [Dim]float32
	BBoxMax     [Dim]float32
}

// Index is the top-level search index backed by an mmap'd file.
type Index struct {
	data        []byte
	file        *os.File
	numVectors  uint32
	numCentroids uint32
	dim         uint16
	scale       uint16

	Centroids  [][Dim]float32
	Partitions []PartitionEntry
	Vectors    [][Dim]float32
	Labels     []uint8
	KDTrees    []*KDTree
}

// LoadIndex mmaps the file at path, parses the header, and builds in-memory
// KD-trees for each non-empty partition. It mlocks the pages and pretouches
// them to ensure they are resident.
func LoadIndex(path string) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	size := fi.Size()
	if size < HeaderSize {
		f.Close()
		return nil, fmt.Errorf("file too small: %d bytes", size)
	}

	data, err := syscall.Mmap(int(f.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("mmap: %w", err)
	}

	// Lock pages in RAM (best-effort; non-fatal on failure).
	_ = syscall.Mlock(data)

	// Pretouch every page to populate the page cache.
	pageSize := os.Getpagesize()
	for off := 0; off < len(data); off += pageSize {
		_ = data[off]
	}

	idx := &Index{
		data: data,
		file: f,
	}

	if err := idx.validateHeader(); err != nil {
		idx.Close()
		return nil, err
	}

	idx.parseArrays()

	if err := idx.buildKDTrees(); err != nil {
		idx.Close()
		return nil, err
	}

	return idx, nil
}

// Close unmaps the file data and closes the underlying file descriptor.
func (idx *Index) Close() {
	if idx.data != nil {
		_ = syscall.Munmap(idx.data)
		idx.data = nil
	}
	if idx.file != nil {
		_ = idx.file.Close()
		idx.file = nil
	}
}

// validateHeader checks the magic, version, and counts stored in the header.
func (idx *Index) validateHeader() error {
	hdr := idx.data[:HeaderSize]

	if string(hdr[0:8]) != Magic {
		return fmt.Errorf("bad magic: %q", string(hdr[0:8]))
	}

	version := binary.LittleEndian.Uint32(hdr[8:12])
	if version != Version {
		return fmt.Errorf("bad version: %d", version)
	}

	idx.numVectors = binary.LittleEndian.Uint32(hdr[12:16])
	if idx.numVectors != NumVectors {
		return fmt.Errorf("bad num_vectors: got %d, want %d",
			idx.numVectors, NumVectors)
	}

	idx.numCentroids = binary.LittleEndian.Uint32(hdr[16:20])
	if idx.numCentroids == 0 || idx.numCentroids > 100000 {
		return fmt.Errorf("bad num_centroids: %d", idx.numCentroids)
	}

	idx.dim = binary.LittleEndian.Uint16(hdr[20:22])
	if idx.dim != Dim {
		return fmt.Errorf("bad dim: got %d, want %d", idx.dim, Dim)
	}

	idx.scale = binary.LittleEndian.Uint16(hdr[22:24])

	return nil
}

// ValidateHeader is the exported wrapper for validateHeader.
func (idx *Index) ValidateHeader() error {
	return idx.validateHeader()
}

// parseArrays overlays Go slices onto the mmap'd byte slice for zero-copy
// access to centroids, partitions, vectors, and labels.
func (idx *Index) parseArrays() {
	nc := int(idx.numCentroids)
	nv := int(idx.numVectors)

	centroidsOff := HeaderSize
	partitionsOff := centroidsOff + nc*Dim*4
	vectorsOff := partitionsOff + nc*int(unsafe.Sizeof(PartitionEntry{}))
	labelsOff := vectorsOff + nv*Dim*4

	idx.Centroids = unsafe.Slice(
		(*[Dim]float32)(unsafe.Pointer(&idx.data[centroidsOff])),
		nc,
	)
	idx.Partitions = unsafe.Slice(
		(*PartitionEntry)(unsafe.Pointer(&idx.data[partitionsOff])),
		nc,
	)
	idx.Vectors = unsafe.Slice(
		(*[Dim]float32)(unsafe.Pointer(&idx.data[vectorsOff])),
		nv,
	)
	idx.Labels = idx.data[labelsOff : labelsOff+nv]
}

// buildKDTrees constructs one KD-tree per non-empty partition.
func (idx *Index) buildKDTrees() error {
	nc := int(idx.numCentroids)
	idx.KDTrees = make([]*KDTree, nc)

	for i := 0; i < nc; i++ {
		p := &idx.Partitions[i]
		if p.Count == 0 {
			continue
		}
		start := int(p.Offset)
		end := start + int(p.Count)
		partVecs := idx.Vectors[start:end]
		partLabels := idx.Labels[start:end]
		tree := BuildKDTree(partVecs, partLabels, LeafSize)
		tree.BaseIdx = int32(p.Offset)
		idx.KDTrees[i] = tree
	}
	return nil
}
