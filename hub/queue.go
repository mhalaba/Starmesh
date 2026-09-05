package hub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type queued struct {
	Env *Envelope `json:"env"`
}

type Queue struct {
	mu    sync.Mutex
	path  string
	max   int
	Items []queued `json:"items"`
}

func OpenQueue(path string, max int) *Queue {
	if max <= 0 {
		max = 1000
	}
	q := &Queue{path: path, max: max}
	b, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(b, q)
	}
	return q
}

func (q *Queue) Push(env *Envelope) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Items = append(q.Items, queued{Env: env})
	if len(q.Items) > q.max {
		q.Items = q.Items[len(q.Items)-q.max:]
	}
	q.saveLocked()
}

func (q *Queue) PopAll() []*Envelope {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]*Envelope, 0, len(q.Items))
	for _, it := range q.Items {
		if it.Env != nil {
			out = append(out, it.Env)
		}
	}
	q.Items = nil
	q.saveLocked()
	return out
}

func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Items)
}

func (q *Queue) saveLocked() {
	if q.path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(q.path), 0o700)
	b, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(q.path, b, 0o600)
}
