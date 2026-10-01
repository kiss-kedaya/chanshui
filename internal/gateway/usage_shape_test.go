package gateway

import "testing"

func TestCompatibleCacheUsageShapes(t *testing.T) {
	cases := []struct {
		name                        string
		usage                       string
		input, read, write          int
		readReported, writeReported bool
	}{
		{
			name:  "responses inclusive cached",
			usage: `{"input_tokens":110,"input_tokens_details":{"cached_tokens":100},"output_tokens":2}`,
			input: 10, read: 100, readReported: true,
		},
		{
			name:  "chat creation inside details",
			usage: `{"prompt_tokens":18444,"completion_tokens":88,"prompt_tokens_details":{"cached_tokens":16384,"cache_creation_tokens":2048}}`,
			input: 12, read: 16384, write: 2048, readReported: true, writeReported: true,
		},
		{
			name:  "responses creation plus cached subset",
			usage: `{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"input_tokens_details":{"cached_tokens":60}}`,
			input: 30, read: 60, write: 10, readReported: true, writeReported: true,
		},
		{
			name:  "write alias outranks creation input",
			usage: `{"prompt_tokens":200,"cache_write_tokens":32,"cache_creation_input_tokens":64}`,
			input: 168, write: 32, writeReported: true,
		},
		{
			name:  "explicit zero write blocks alias",
			usage: `{"input_tokens":200,"input_tokens_details":{"cache_write_tokens":0,"cache_creation_tokens":64},"cache_creation_input_tokens":64}`,
			input: 200, write: 0, writeReported: true,
		},
		{
			name:  "explicit zero read blocks alias",
			usage: `{"prompt_tokens":200,"input_tokens_details":{"cached_tokens":0},"prompt_tokens_details":{"cached_tokens":128},"cached_tokens":64}`,
			input: 200, read: 0, readReported: true,
		},
		{
			name:  "deepseek hit and miss",
			usage: `{"prompt_tokens":300,"prompt_cache_hit_tokens":120,"prompt_cache_miss_tokens":40}`,
			input: 140, read: 120, write: 40, readReported: true, writeReported: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var m Metrics
			raw := []byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":` + tc.usage + `}`)
			if err := parseCompatibleJSON(raw, "chat", &m); err != nil {
				t.Fatal(err)
			}
			if m.InputTokens != tc.input || m.CacheRead != tc.read || m.CacheCreation5m+m.CacheCreation1h != tc.write || m.CacheReadReported != tc.readReported || m.CacheWriteReported != tc.writeReported {
				t.Fatalf("got input=%d read=%d write=%d readSet=%v writeSet=%v", m.InputTokens, m.CacheRead, m.CacheCreation5m+m.CacheCreation1h, m.CacheReadReported, m.CacheWriteReported)
			}
		})
	}
}
