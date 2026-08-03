package async

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestQueueDequeuePriorityAndFIFO(t *testing.T) {
	queue := NewQueue[string]()
	for _, entry := range []struct {
		item     string
		priority Priority
	}{
		{item: "low-1", priority: PriorityLow},
		{item: "high-1", priority: PriorityHigh},
		{item: "low-2", priority: PriorityLow},
		{item: "high-2", priority: PriorityHigh},
	} {
		if err := queue.Enqueue(entry.item, entry.priority); err != nil {
			t.Fatalf("Enqueue(%q) error = %v", entry.item, err)
		}
	}

	high, low := queue.Pending()
	if high != 2 || low != 2 {
		t.Fatalf("Pending() = (%d, %d), want (2, 2)", high, low)
	}

	var got []string
	for {
		item, ok := queue.Dequeue()
		if !ok {
			break
		}
		got = append(got, item)
	}
	want := []string{"high-1", "high-2", "low-1", "low-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dequeue order = %v, want %v", got, want)
	}
}

func TestQueueRemoveRequeueAndClose(t *testing.T) {
	queue := NewQueue[string]()
	if err := queue.Enqueue("low-1", PriorityLow); err != nil {
		t.Fatal(err)
	}
	if err := queue.Enqueue("low-2", PriorityLow); err != nil {
		t.Fatal(err)
	}
	if !queue.Remove("low-1") {
		t.Fatal("Remove(low-1) = false, want true")
	}
	if queue.Remove("missing") {
		t.Fatal("Remove(missing) = true, want false")
	}
	if !queue.RequeueFront("high", PriorityHigh) {
		t.Fatal("RequeueFront(high) = false, want true")
	}
	if !queue.HasPriority(PriorityHigh) {
		t.Fatal("HasPriority(PriorityHigh) = false, want true")
	}

	pending, alreadyClosed := queue.Close()
	if alreadyClosed {
		t.Fatal("first Close() reported already closed")
	}
	if want := []string{"high", "low-2"}; !reflect.DeepEqual(pending, want) {
		t.Fatalf("Close() pending = %v, want %v", pending, want)
	}
	if err := queue.Enqueue("late", PriorityHigh); !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("Enqueue after close error = %v, want %v", err, ErrQueueClosed)
	}
	if queue.RequeueFront("late", PriorityHigh) {
		t.Fatal("RequeueFront after close = true, want false")
	}
	if _, alreadyClosed = queue.Close(); !alreadyClosed {
		t.Fatal("second Close() did not report already closed")
	}
}

func TestQueueWorkerLifecycle(t *testing.T) {
	queue := NewQueue[int]()
	started := make(chan struct{})
	queue.Start(func() {
		<-queue.Wake()
		close(started)
		<-queue.Context().Done()
	})
	queue.Signal()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not receive wake signal")
	}
	queue.Close()
	select {
	case <-queue.Done():
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after close")
	}
}
