package main

import "testing"

func TestAppendSecretPaste(t *testing.T) {
	tests := []struct {
		name   string
		buf    string
		pasted string
		want   string
	}{
		{name: "plain", buf: "", pasted: "sk-abc", want: "sk-abc"},
		{name: "trim trailing newline", buf: "", pasted: "sk-abc\n", want: "sk-abc"},
		{name: "trim crlf", buf: "", pasted: "sk-abc\r\n", want: "sk-abc"},
		{name: "append to existing", buf: "pre-", pasted: "post\n", want: "pre-post"},
		{name: "keep internal newline", buf: "", pasted: "a\nb\n", want: "a\nb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendSecretPaste(tt.buf, tt.pasted)
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}
