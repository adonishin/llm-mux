// Package ir provides memory pools for the translator layer.
package ir

import (
	"bytes"
	"strings"
	"sync"
)

var BytesBufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 1024))
	},
}

// GetBuffer retrieves a buffer from the pool.
func GetBuffer() *bytes.Buffer {
	return BytesBufferPool.Get().(*bytes.Buffer)
}

// PutBuffer returns a buffer to the pool after resetting it.
func PutBuffer(buf *bytes.Buffer) {
	buf.Reset()
	BytesBufferPool.Put(buf)
}

// StringBuilderPool provides reusable strings.Builder instances.
var StringBuilderPool = sync.Pool{
	New: func() any {
		b := &strings.Builder{}
		b.Grow(512)
		return b
	},
}

// GetStringBuilder retrieves a string builder from the pool.
func GetStringBuilder() *strings.Builder {
	return StringBuilderPool.Get().(*strings.Builder)
}

// PutStringBuilder returns a string builder to the pool after resetting it.
func PutStringBuilder(sb *strings.Builder) {
	sb.Reset()
	StringBuilderPool.Put(sb)
}

// anySlicePool provides reusable []any slices for building JSON arrays.
var anySlicePool = sync.Pool{
	New: func() any {
		s := make([]any, 0, 16)
		return &s
	},
}

// GetAnySlice retrieves a []any slice from the pool with the given capacity hint.
func GetAnySlice(capHint int) []any {
	sp := anySlicePool.Get().(*[]any)
	s := *sp
	if cap(s) < capHint {
		s = make([]any, 0, capHint)
	}
	return s[:0]
}

// PutAnySlice returns a []any slice to the pool.
func PutAnySlice(s []any) {
	// Clear references to help GC
	for i := range s {
		s[i] = nil
	}
	s = s[:0]
	anySlicePool.Put(&s)
}

// stringSlicePool provides reusable []string slices.
var stringSlicePool = sync.Pool{
	New: func() any {
		s := make([]string, 0, 8)
		return &s
	},
}

// GetStringSlice retrieves a []string slice from the pool.
func GetStringSlice(capHint int) []string {
	sp := stringSlicePool.Get().(*[]string)
	s := *sp
	if cap(s) < capHint {
		s = make([]string, 0, capHint)
	}
	return s[:0]
}

// PutStringSlice returns a []string slice to the pool.
func PutStringSlice(s []string) {
	for i := range s {
		s[i] = ""
	}
	s = s[:0]
	stringSlicePool.Put(&s)
}

// -----------------------------------------------------------------------------
// Map Pools - Reduce allocations for common map types
// -----------------------------------------------------------------------------

// mapPool provides reusable map[string]any for JSON object building.
var mapPool = sync.Pool{
	New: func() any {
		return make(map[string]any, 8)
	},
}

// GetMap retrieves a map from the pool.
func GetMap() map[string]any {
	return mapPool.Get().(map[string]any)
}

// PutMap returns a map to the pool after clearing it.
func PutMap(m map[string]any) {
	clear(m)
	mapPool.Put(m)
}

// -----------------------------------------------------------------------------
// UUID Pool - Optimized UUID generation
// -----------------------------------------------------------------------------

// uuidBytePool provides reusable byte slices for UUID generation.
var uuidBytePool = sync.Pool{
	New: func() any {
		b := make([]byte, 16)
		return &b
	},
}

// GetUUIDBuf retrieves a 16-byte buffer for UUID generation.
func GetUUIDBuf() *[]byte {
	return uuidBytePool.Get().(*[]byte)
}

// PutUUIDBuf returns a UUID buffer to the pool.
func PutUUIDBuf(b *[]byte) {
	uuidBytePool.Put(b)
}

// -----------------------------------------------------------------------------
// Pre-allocated common values
// -----------------------------------------------------------------------------

// Common empty values to avoid allocations
var (
	EmptyMap       = map[string]any{}
	EmptySlice     = []any{}
	EmptyStringMap = map[string]string{}
)

// JSON Schema version constants
// Claude API requires JSON Schema draft 2020-12
// See: https://docs.anthropic.com/en/docs/build-with-claude/tool-use
const (
	JSONSchemaDraft202012 = "https://json-schema.org/draft/2020-12/schema"
)

// Common JSON schema fragments (immutable, safe to share)
var (
	EmptyObjectSchema = map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}

	ClaudeEmptyInputSchema = map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
		"$schema":              JSONSchemaDraft202012,
	}
)

// -----------------------------------------------------------------------------
// SSE Chunk Pools - Optimized for streaming responses
// -----------------------------------------------------------------------------

// sseChunkPool provides reusable byte slices for SSE chunk building.
var sseChunkPool = sync.Pool{
	New: func() any {
		// Typical SSE chunk: "data: {...}\n\n" - allocate 512 bytes
		b := make([]byte, 0, 512)
		return &b
	},
}

// GetSSEChunkBuf retrieves a buffer for SSE chunk building.
func GetSSEChunkBuf() []byte {
	bp := sseChunkPool.Get().(*[]byte)
	return (*bp)[:0]
}

// PutSSEChunkBuf returns an SSE chunk buffer to the pool.
func PutSSEChunkBuf(b []byte) {
	if cap(b) >= 512 && cap(b) <= 4096 {
		bp := b[:0]
		sseChunkPool.Put(&bp)
	}
}

// BuildSSEChunk builds an SSE chunk with "data: " prefix efficiently.
// Returns a pooled buffer - caller should call PutSSEChunkBuf when done.
func BuildSSEChunk(jsonData []byte) []byte {
	size := 6 + len(jsonData) + 2 // "data: " + json + "\n\n"
	buf := GetSSEChunkBuf()
	if cap(buf) < size {
		buf = make([]byte, 0, size)
	}
	buf = append(buf, "data: "...)
	buf = append(buf, jsonData...)
	buf = append(buf, "\n\n"...)
	return buf
}

// BuildSSEEvent builds an SSE event with event type and data.
// Format: "event: <type>\ndata: <json>\n\n"
func BuildSSEEvent(eventType string, jsonData []byte) []byte {
	size := 7 + len(eventType) + 7 + len(jsonData) + 2
	buf := GetSSEChunkBuf()
	if cap(buf) < size {
		buf = make([]byte, 0, size)
	}
	buf = append(buf, "event: "...)
	buf = append(buf, eventType...)
	buf = append(buf, "\ndata: "...)
	buf = append(buf, jsonData...)
	buf = append(buf, "\n\n"...)
	return buf
}
