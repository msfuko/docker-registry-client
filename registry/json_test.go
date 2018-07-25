package registry

import (
	"net/http"
	"testing"
)

func Test_GetNextLink(t *testing.T) {
	tcs := []struct {
		name     string
		header   []string
		expected string
	}{
		{
			name:     "dtr pagination",
			header:   []string{`<?start=0>; rel="start", <?start=39>; rel="next"`},
			expected: `https://example.com?start=39`,
		},
		{
			name:     "dtr pagination",
			header:   []string{`<?start=0>; rel="start", <?start=39>; type="application/json"; rel="next"`},
			expected: `https://example.com?start=39`,
		},
		{
			name:     "dtr pagination with separate headers",
			header:   []string{`<?start=0>; rel="start"`, `<?start=39>; type="application/json"; rel="next"`},
			expected: `https://example.com?start=39`,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := getNextLink("https://example.com", &http.Response{
				Header: http.Header{
					"Link": tc.header,
				},
			})
			if tc.expected != s {
				t.Errorf("Expected %v, got %v", tc.expected, s)
			}
		})
	}
}
