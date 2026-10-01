package store

import "sync"

type RecordLocation struct {
	File   string
	Offset int64
	Length int64
}

type Index struct {
	mu      sync.RWMutex
	records map[string]RecordLocation
	order   []string
}

func NewIndex() *Index {
	return &Index{
		records: make(map[string]RecordLocation),
		order:   make([]string, 0),
	}
}

func (i *Index) Set(id string, location RecordLocation) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if _, exists := i.records[id]; !exists {
		i.order = append(i.order, id)
	}
	i.records[id] = location
}

func (i *Index) Get(id string) (RecordLocation, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	location, ok := i.records[id]
	return location, ok
}

func (i *Index) IDs() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()

	n := len(i.order)
	res := make([]string, n)
	for idx, id := range i.order {
		res[n-1-idx] = id
	}
	return res
}

func (i *Index) Len() int {
	i.mu.RLock()
	defer i.mu.RUnlock()

	return len(i.order)
}

func (i *Index) RemoveFile(file string) {
	i.mu.Lock()
	defer i.mu.Unlock()

	newOrder := make([]string, 0, len(i.order))
	for _, id := range i.order {
		loc := i.records[id]
		if loc.File == file {
			delete(i.records, id)
		} else {
			newOrder = append(newOrder, id)
		}
	}
	i.order = newOrder
}

func (i *Index) Clear() {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.records = make(map[string]RecordLocation)
	i.order = make([]string, 0)
}
