package utils

import (
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// UniqueNamer ensures generated filenames remain unique by appending counters.
type UniqueNamer struct {
	mu     sync.Mutex
	counts map[string]int
}

// NewUniqueNamer returns an initialized UniqueNamer.
func NewUniqueNamer() *UniqueNamer {
	return &UniqueNamer{
		counts: make(map[string]int),
	}
}

// Reserve records a filename and returns a unique variant if needed.
func (n *UniqueNamer) Reserve(filename string) string {
	n.mu.Lock()
	defer n.mu.Unlock()

	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	ext := filepath.Ext(filename)
	count := n.counts[base]

	if count == 0 {
		n.counts[base] = 1
		return filename
	}

	n.counts[base] = count + 1
	return base + "-" + strconv.Itoa(count) + ext
}
