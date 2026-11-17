package mcp

import "fmt"

// ServerStatus is the outcome of connecting one MCP server.
type ServerStatus struct {
	ID        string
	OK        bool
	ToolCount int
	Skipped   bool // optional soft-fail
	Err       string
}

// ConnectReport summarizes ConnectAll.
type ConnectReport struct {
	Servers []ServerStatus
}

// Lines returns human-readable status lines for the host UI.
func (r ConnectReport) Lines() []string {
	if len(r.Servers) == 0 {
		return nil
	}
	out := make([]string, 0, len(r.Servers))
	for _, s := range r.Servers {
		switch {
		case s.OK:
			out = append(out, fmt.Sprintf("mcp: %s ok (%d tools)", s.ID, s.ToolCount))
		case s.Skipped:
			msg := s.Err
			if msg == "" {
				msg = "unknown error"
			}
			out = append(out, fmt.Sprintf("mcp: skipped %s (%s)", s.ID, msg))
		default:
			msg := s.Err
			if msg == "" {
				msg = "failed"
			}
			out = append(out, fmt.Sprintf("mcp: %s failed (%s)", s.ID, msg))
		}
	}
	return out
}
