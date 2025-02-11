package dial

import (
	"container/heap"
	"sync"

	"github.com/0xPolygon/polygon-edge/network/common"

	"github.com/libp2p/go-libp2p/core/peer"
)

const updateChBufferSize = 20

// DialQueue is a queue that holds dials tasks for potential peers, implemented as a min-heap
type DialQueue struct {
	sync.Mutex

	heap  dialQueueImpl
	tasks map[peer.ID]*DialTask

	updateCh chan struct{}
	closeCh  chan struct{}
}

// NewDialQueue creates a new DialQueue instance
func NewDialQueue() *DialQueue {
	return &DialQueue{
		heap:     dialQueueImpl{},
		tasks:    map[peer.ID]*DialTask{},
		updateCh: make(chan struct{}, updateChBufferSize),
		closeCh:  make(chan struct{}),
	}
}

// Close closes the running DialQueue
func (d *DialQueue) Close() {
	close(d.closeCh)
}

// PopTask is a loop that handles update and close events [BLOCKING]
func (d *DialQueue) PopTask() *DialTask {
	for {
		select {
		case <-d.updateCh: // blocks until AddTask is called...
			if task := d.popTaskImpl(); task != nil {
				return task
			}
		case <-d.closeCh: // ... or dial queue is closed
			return nil
		}
	}
}

// popTaskImpl is the implementation for task popping from the min-heap
func (d *DialQueue) popTaskImpl() *DialTask {
	d.Lock()
	defer d.Unlock()

	if len(d.heap) != 0 {
		// pop the first value and remove it from the heap
		task, ok := heap.Pop(&d.heap).(*DialTask)
		if !ok {
			return nil
		}

		delete(d.tasks, task.addrInfo.ID)

		return task
	}

	return nil
}

// DeleteTask deletes a task from the dial queue for the specified peer
func (d *DialQueue) DeleteTask(peer peer.ID) {
	d.Lock()
	defer d.Unlock()

	item, ok := d.tasks[peer]
	if ok {
		heap.Remove(&d.heap, item.index)
		delete(d.tasks, peer)
	}
}

// AddTask adds a new task to the dial queue
func (d *DialQueue) AddTask(addrInfo *peer.AddrInfo, priority common.DialPriority) {
	if d.addTaskImpl(addrInfo, priority) {
		select {
		case <-d.closeCh:
		case d.updateCh <- struct{}{}:
		}
	}
}

func (d *DialQueue) addTaskImpl(addrInfo *peer.AddrInfo, priority common.DialPriority) bool {
	d.Lock()
	defer d.Unlock()

	// do not spam queue with same addresses
	if item, ok := d.tasks[addrInfo.ID]; ok {
		// if existing priority greater than new one, replace item
		if item.priority > uint64(priority) {
			item.addrInfo = addrInfo
			item.priority = uint64(priority)
			heap.Fix(&d.heap, item.index)
		}

		return false
	}

	task := &DialTask{
		addrInfo: addrInfo,
		priority: uint64(priority),
	}
	d.tasks[addrInfo.ID] = task
	heap.Push(&d.heap, task)

	return true
}
