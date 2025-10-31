package lib

import (
	"fmt"
	"hash/fnv"
)

type Sharder[K comparable, S any] struct {
	shards    []S
	numShards int
}

func NewSharder[K comparable, S any](shards []S) *Sharder[K, S] {
	if len(shards) == 0 {
		panic("at least one shard required")
	}

	return &Sharder[K, S]{
		shards:    shards,
		numShards: len(shards),
	}
}

func (s *Sharder[K, S]) GetShard(key K) S {
	hash := s.hash(key)
	return s.shards[hash%uint32(s.numShards)]
}

func (s *Sharder[K, S]) GetShardByIndex(index int) S {
	if index < 0 || index >= s.numShards {
		panic(fmt.Sprintf("index %d out of range [0, %d)", index, s.numShards))
	}
	return s.shards[index]
}

func (s *Sharder[K, S]) NumShards() int {
	return s.numShards
}

func (s *Sharder[K, S]) ForEachShard(fn func(index int, shard S)) {
	for i, shard := range s.shards {
		fn(i, shard)
	}
}

func (s *Sharder[K, S]) hash(key K) uint32 {
	h := fnv.New32a()
	fmt.Fprintf(h, "%v", key)
	return h.Sum32()
}
