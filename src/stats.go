package main

import (
	"time"
)

type Stats struct {
	Avg_latency   int64
	Worst_latency int64

	Deltas_all      [10]int64
	Deltas_len      int
	Deltas_last_idx int
}

var stats = Stats{
	Deltas_len: 0,
}

func (s *Stats) appendMessage(m *Message) {
	now := time.Now().UnixMilli()

	s.Deltas_all[s.Deltas_last_idx] = now - m.Timestamp

	s.Deltas_last_idx = (s.Deltas_last_idx + 1) % len(s.Deltas_all)
	s.Deltas_len = min(s.Deltas_len+1, len(s.Deltas_all))

	sum := int64(0)
	s.Worst_latency = 0
	for i := range s.Deltas_len {
		sum += s.Deltas_all[i]
		s.Worst_latency = max(s.Deltas_all[i], s.Deltas_all[0])
	}

	s.Avg_latency = sum / int64(s.Deltas_len)
}
