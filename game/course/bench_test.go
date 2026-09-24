package course

import "testing"

func BenchmarkChunk(b *testing.B) {
	c := New(1)
	for i := 0; i < b.N; i++ {
		c.Chunk(i % 50)
	}
}
