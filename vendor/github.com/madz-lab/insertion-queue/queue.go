package queue

// Queue is the priority queue based on insertion sort
type Queue []Item

// NewQueue creates an instance of the priority queue
func NewQueue() Queue {
	return make(Queue, 0)
}

// Len returns the length of the queue
func (q *Queue) Len() int {
	return len(*q)
}

// Index returns the element at the specified index, if any.
// NOTE: panics if out of bounds
func (q *Queue) Index(index int) Item {
	return (*q)[index]
}

// Push adds a new element to the priority queue
func (q *Queue) Push(item Item) {
	*q = append(*q, item)
	for i := len(*q) - 1; i > 0; i-- {
		if (*q)[i].Less((*q)[i-1]) {
			(*q)[i], (*q)[i-1] = (*q)[i-1], (*q)[i]
		} else {
			// queue is sorted, no need to continue iteration
			break
		}
	}
}

// Fix makes sure the priority queue is properly sorted
func (q *Queue) Fix() {
	for i := 1; i < len(*q); i++ {
		for j := i - 1; j >= 0; j-- {
			if (*q)[j].Less((*q)[j+1]) {
				break
			}

			(*q)[j], (*q)[j+1] = (*q)[j+1], (*q)[j]
		}
	}
}

// PopFront removes the first element in the queue, if any
func (q *Queue) PopFront() Item {
	if len(*q) == 0 {
		return nil
	}

	el := (*q)[0]
	*q = (*q)[1:]

	return el
}

// PopBack removes the last element in the queue, if any
func (q *Queue) PopBack() Item {
	if len(*q) == 0 {
		return nil
	}

	el := (*q)[len(*q)-1]
	*q = (*q)[:len(*q)-1]

	return el
}
