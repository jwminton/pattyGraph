// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import "sync"

// ringSeriesAccumulator is a GC-free, reusable time-series buffer with tail-aligned merge support.
// It is intended for use in high-throughput systems where allocations must be tightly controlled.
// ringBuffers know how to wrap but they're terrible at aggregation
type ringSeriesAccumulator struct {
	data  [80]int
	len   int // logical length (<= len(data))
	index int // write head or alignment offset if needed
}

// Reset zeroes out the ringSeriesAccumulator and prepares it for reuse.
func (w *ringSeriesAccumulator) Reset() {
	for i := 0; i < w.len; i++ {
		w.data[i] = 0
	}
	w.len = 0
	w.index = 0
}

// Len returns the current logical length of the buffer.
func (w *ringSeriesAccumulator) Len() int {
	return w.len
}

// unsafeAt returns the value at logical index i (0-based).
func (w *ringSeriesAccumulator) unsafeAt(i int) int {
	if i < 0 || i >= w.len {
		panic("ringSeriesAccumulator.unsafeAt: out of bounds")
	}
	return w.data[i]
}

// unsafeAddAt adds v to value at logical index i.
func (w *ringSeriesAccumulator) unsafeAddAt(i int, v int) {
	if i < 0 || i >= w.len {
		panic("ringSeriesAccumulator.unsafeAddAt: out of bounds")
	}
	w.data[i] += v
}

// CloneInto copies the contents of w into dst.
func (w *ringSeriesAccumulator) CloneInto(dst *ringSeriesAccumulator) {
	copy(dst.data[:], w.data[:w.len])
	dst.len = w.len
	dst.index = w.index
}

// EnsureCapacityMatch ensures that w.Len() >= src.Len() by front-padding with zeros if needed.
func (w *ringSeriesAccumulator) EnsureCapacityMatch(src *ringSeriesAccumulator) {
	if w.len < src.len {
		diff := src.len - w.len
		// shift right by diff
		for i := w.len - 1; i >= 0; i-- {
			w.data[i+diff] = w.data[i]
		}
		for i := 0; i < diff; i++ {
			w.data[i] = 0
		}
		w.len = src.len
	}
}

// MergeFrom adds src into w tail-aligned.
func (w *ringSeriesAccumulator) MergeFrom(src *ringSeriesAccumulator) {
	offset := w.len - src.len
	for i := 0; i < src.len; i++ {
		w.data[offset+i] += src.data[i]
	}
}
func getAccumulatorBuffer() *ringSeriesAccumulator {
	//windowGets++
	r := windowBufferPool.Get().(*ringSeriesAccumulator)
	return r
}
func poolAccumulator(r *ringSeriesAccumulator) {
	r.Reset()
	//windowReturns++
	windowBufferPool.Put(r)
}

var windowBufferPool = sync.Pool{
	New: func() any {
		//windowNews++
		return &ringSeriesAccumulator{}
	},
}

func accumulatorFor(src *ringBuffer) *ringSeriesAccumulator {
	//var w *ringSeriesAccumulator
	w := getAccumulatorBuffer()
	srcLen := src.Len()
	// Copy ring data into flat view of the windowed buffer
	for i := 0; i < srcLen; i++ {
		w.data[i] = src.unsafeAt(i) // At() handles wraparound safely
	}
	w.len = srcLen
	w.index = 0 // or preserve if you start using it semantically
	return w
}

func (w *ringSeriesAccumulator) MergeFromRing(src *ringBuffer) {
	srcLen := src.Len()
	if srcLen == 0 {
		return // nothing to merge
	}

	// Extend accumulator window if needed
	if w.len < srcLen {
		diff := srcLen - w.len
		for i := w.len - 1; i >= 0; i-- {
			w.data[i+diff] = w.data[i]
		}
		for i := 0; i < diff; i++ {
			w.data[i] = 0
		}
		w.len = srcLen
	}

	offset := w.len - srcLen

	// Fast path: use flat view if possible
	if view := src.FlatSlice(); view != nil {
		for i, val := range view {
			w.data[offset+i] += val
		}
		return
	}

	// Fallback: use .At() for wrapped ring
	for i := 0; i < srcLen; i++ {
		w.data[offset+i] += src.unsafeAt(i)
	}
}

func (w *ringSeriesAccumulator) cloneInto(r *ringBuffer) {
	for i := 0; i < w.len; i++ {
		r.data[i] = w.data[i]
	}
	r.size = w.len
	r.index = 0 // assume canonical start
}

// Slice returns a deep‐copied snapshot of the buffer contents.
func (w *ringSeriesAccumulator) Slice() []int {
	out := make([]int, w.len)
	copy(out, w.data[:w.len])
	return out
}

// Slice returns a forward-facing view of the buffer contents as a slice.
func (w *ringSeriesAccumulator) OldSlice() []int {
	return w.data[:w.len]
}

// ReverseSlice returns a reversed slice of the buffer contents.
// This allocates a new slice (used only for display, so GC is acceptable).
func (w *ringSeriesAccumulator) ReverseSlice() []int {
	reversed := make([]int, w.len)
	for i := 0; i < w.len; i++ {
		reversed[i] = w.data[w.len-1-i]
	}
	return reversed
}
