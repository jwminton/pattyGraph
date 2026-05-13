// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"math"
	"reflect"
	"testing"
)

func TestAccumulatorForCopiesRingBufferInLogicalOrder(t *testing.T) {
	var r ringBuffer
	for i := 1; i <= 85; i++ {
		r.Push(i)
	}

	acc := accumulatorFor(&r)
	defer poolAccumulator(acc)

	if acc.Len() != 80 {
		t.Fatalf("Len() = %d, want 80", acc.Len())
	}
	if got := acc.Slice(); got[0] != 6 || got[len(got)-1] != 85 {
		t.Fatalf("Slice first/last = %d/%d, want 6/85", got[0], got[len(got)-1])
	}
}

func TestRingSeriesAccumulatorMergeFromRingTailAligns(t *testing.T) {
	var long ringBuffer
	for _, v := range []int{1, 2, 3, 4} {
		long.Push(v)
	}

	var short ringBuffer
	for _, v := range []int{10, 20} {
		short.Push(v)
	}

	acc := accumulatorFor(&long)
	defer poolAccumulator(acc)
	acc.MergeFromRing(&short)

	want := []int{1, 2, 13, 24}
	if got := acc.Slice(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Slice() = %v, want %v", got, want)
	}
}

func TestRingSeriesAccumulatorMergeFromRingExtendsAndTailAligns(t *testing.T) {
	var short ringBuffer
	for _, v := range []int{10, 20} {
		short.Push(v)
	}
	acc := accumulatorFor(&short)
	defer poolAccumulator(acc)

	var long ringBuffer
	for _, v := range []int{1, 2, 3, 4} {
		long.Push(v)
	}
	acc.MergeFromRing(&long)

	want := []int{1, 2, 13, 24}
	if got := acc.Slice(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Slice() = %v, want %v", got, want)
	}
}

func TestRingSeriesAccumulatorReverseSlice(t *testing.T) {
	var r ringBuffer
	for _, v := range []int{1, 2, 3} {
		r.Push(v)
	}

	acc := accumulatorFor(&r)
	defer poolAccumulator(acc)

	want := []int{3, 2, 1}
	if got := acc.ReverseSlice(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ReverseSlice() = %v, want %v", got, want)
	}
}

func TestRingBufferPushAndRead(t *testing.T) {
	var r ringBuffer
	r.Push(10)
	r.Push(20)
	r.Push(30)

	if r.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", r.Len())
	}
	if r.At(0) != 10 || r.At(1) != 20 || r.At(2) != 30 {
		t.Fatalf("At values = %d,%d,%d; want 10,20,30", r.At(0), r.At(1), r.At(2))
	}
	if r.Latest() != 30 {
		t.Fatalf("Latest() = %d, want 30", r.Latest())
	}
	if r.Penultimate() != 20 {
		t.Fatalf("Penultimate() = %d, want 20", r.Penultimate())
	}
}

func TestRingBufferWraparoundKeepsLast80Values(t *testing.T) {
	var r ringBuffer
	for i := 1; i <= 85; i++ {
		r.Push(i)
	}

	if r.Len() != 80 {
		t.Fatalf("Len() = %d, want 80", r.Len())
	}
	if r.At(0) != 6 {
		t.Fatalf("At(0) = %d, want 6", r.At(0))
	}
	if r.Latest() != 85 {
		t.Fatalf("Latest() = %d, want 85", r.Latest())
	}
	if r.Penultimate() != 84 {
		t.Fatalf("Penultimate() = %d, want 84", r.Penultimate())
	}
}

func TestRingBufferFlux(t *testing.T) {
	var r ringBuffer
	for i := 1; i <= 5; i++ {
		r.Push(i)
	}

	if got := r.nFlux(3); got != 12 {
		t.Fatalf("nFlux(3) = %d, want 12", got)
	}
	if got := r.nFlux(99); got != 15 {
		t.Fatalf("nFlux(99) = %d, want 15", got)
	}
	if got := r.nFluxAvg(4); math.Abs(got-3.5) > 0.000001 {
		t.Fatalf("nFluxAvg(4) = %f, want 3.5", got)
	}
}

func TestRingBufferReset(t *testing.T) {
	var r ringBuffer
	r.Push(1)
	r.Push(2)
	r.Reset()

	if r.Len() != 0 {
		t.Fatalf("Len() after Reset = %d, want 0", r.Len())
	}
	if r.Latest() != 0 {
		t.Fatalf("Latest() after Reset = %d, want 0", r.Latest())
	}
}
