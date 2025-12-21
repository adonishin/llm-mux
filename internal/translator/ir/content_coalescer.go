package ir

import "sync"

type geminiContent struct {
	role  string
	parts []any
}

// ContentCoalescer merges consecutive same-role contents efficiently.
type ContentCoalescer struct {
	contents []geminiContent
	lastRole string
}

var contentCoalescerPool = sync.Pool{
	New: func() any {
		return &ContentCoalescer{contents: make([]geminiContent, 0, 16)}
	},
}

func GetContentCoalescer(capacity int) *ContentCoalescer {
	c := contentCoalescerPool.Get().(*ContentCoalescer)
	if cap(c.contents) < capacity {
		c.contents = make([]geminiContent, 0, capacity)
	}
	return c
}

func PutContentCoalescer(c *ContentCoalescer) {
	c.contents = c.contents[:0]
	c.lastRole = ""
	contentCoalescerPool.Put(c)
}

func (c *ContentCoalescer) Emit(role string, parts []any) {
	if len(parts) == 0 {
		return
	}
	if role == c.lastRole && len(c.contents) > 0 {
		last := &c.contents[len(c.contents)-1]
		last.parts = append(last.parts, parts...)
		return
	}
	c.contents = append(c.contents, geminiContent{role: role, parts: parts})
	c.lastRole = role
}

func (c *ContentCoalescer) Build() []any {
	if len(c.contents) == 0 {
		return nil
	}
	result := make([]any, len(c.contents))
	for i := range c.contents {
		result[i] = map[string]any{
			"role":  c.contents[i].role,
			"parts": c.contents[i].parts,
		}
	}
	return result
}
