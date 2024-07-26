## Overview

[![codecov](https://codecov.io/gh/madz-lab/insertion-queue/branch/main/graph/badge.svg?token=UJW1HMBFUM)](https://codecov.io/gh/madz-lab/insertion-queue)

`insertion-queue` is a small library that implements a priority queue in slice representation, where the order of items
is sorted using insertion sort.

## Usage

### Installation

To start using it, fetch it using `go get`:

```bash
go get github.com/madz-lab/insertion-queue
```

### Basic operations

In order to use the priority queue with custom items, users need to define a special type that implements the methods in
the `Item` interface (`Less`):

```go
package main

import (
	"fmt"

	iq "github.com/madz-lab/insertion-queue"
)

// number is a wrapper for a queue number value
type number struct {
	value int
}

// Less is a required method implementation from iq.Item
func (i number) Less(other iq.Item) bool {
	return i.value < other.(number).value
}

func main() {
	// Creating the queue
	q := iq.NewQueue()

	// Adding an item to the queue
	q.Push(number{1}) // [1]
	q.Push(number{2}) // [1, 2]

	// Indexing an element
	fmt.Println(q[0])

	// Popping an item from the front
	front := q.PopFront()

	// Popping an item from the back
	back := q.PopBack()

	// Fetching the size
	fmt.Println(len(q))
}

```

## Why would anyone use this?

This is a fair question - users might be tempted to simply wrap a slice
within their own structure, and define an insertion method that:

- appends an item to the end of the queue
- calls `sort.Sort` to sort the entire queue

It turns out this is not really efficient, especially if the use-case of the queue
is that items will be added sequentially (one-by-one). Insertion sort provides the biggest performance
benefits in data sets that are _nearly_ sorted. Given that each element being added to the `insertion-queue` is
automatically sorted on insertion, the benefit is obvious - the overhead for placing the new element is minimal.

Additionally, this package is for users who want to have control over their input set (slice), by being able to directly
interact with it (index it and change values directly). When using a structure like `container/heap`, the order of items
within the slice cannot be guaranteed after pushes and pops, and this can pose a problem if users plan to index the
slice
right away after modifying its internal data set.

This package outperforms the standard library implementation mentioned in the bullet points:

```bash
==================

Items: 100
Iterations: 100
Name             Time [s]
insertion-queue  0.00137
stdlib           0.00404

insertion-queue is faster by 0.00267s

==================

Items: 1000
Iterations: 100
Name             Time [s]
insertion-queue  0.13429
stdlib           0.41784

insertion-queue is faster by 0.28356s
```

The snippet for this performance test can be
found [here](https://gist.github.com/zivkovicmilos/ce12d68304e0aa7502f8f7173341821b).

## Benchmarks

```bash
goos: darwin
goarch: amd64
pkg: github.com/madz-lab/insertion-queue
cpu: Intel(R) Core(TM) i5-1038NG7 CPU @ 2.00GHz
BenchmarkHeap_Push10-8           2443876               410.9 ns/op           496 B/op          5 allocs/op
BenchmarkHeap_Push100-8            77829             14067 ns/op            4080 B/op          8 allocs/op
BenchmarkHeap_PopFront10-8       3322941               379.2 ns/op            24 B/op          1 allocs/op
BenchmarkHeap_PopFront100-8      1863390               624.2 ns/op            24 B/op          1 allocs/op
```
