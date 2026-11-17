package main

import "github.com/kasra-sh/zendawn-go/session"

var modeOrder = []string{session.ModeAsk, session.ModePlan, session.ModeAgent}

func nextMode(cur string) string {
	for i, m := range modeOrder {
		if m == cur {
			return modeOrder[(i+1)%len(modeOrder)]
		}
	}
	return session.ModeAgent
}

func modeLabel(m string) string {
	switch m {
	case session.ModeAsk:
		return "ask"
	case session.ModePlan:
		return "plan"
	default:
		return "agent"
	}
}
