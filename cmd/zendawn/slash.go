package main

import (
	"strings"
)

const slashSkillUsage = "usage: /<skill-id> | /skills | /skill load <id> | /skill unload <id>"

// slashSkillCmd is a parsed host slash skill command.
type slashSkillCmd struct {
	list   bool
	unload bool
	name   string // skill id query (may need ResolveName)
	usage  bool   // incomplete /skill form — show usage
	ok     bool   // true if this input is host-handled (never submit to model)
}

// parseSlashSkill returns a host command when the input should not go to the model.
// Any non-empty line starting with / is host-only.
func parseSlashSkill(text string) slashSkillCmd {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return slashSkillCmd{}
	}
	body := strings.TrimSpace(strings.TrimPrefix(text, "/"))
	if body == "" {
		return slashSkillCmd{ok: true, usage: true}
	}
	parts := strings.Fields(body)

	if strings.EqualFold(parts[0], "skills") {
		return slashSkillCmd{ok: true, list: true}
	}

	if strings.EqualFold(parts[0], "skill") {
		switch len(parts) {
		case 1:
			return slashSkillCmd{ok: true, usage: true}
		case 2:
			if strings.EqualFold(parts[1], "load") || strings.EqualFold(parts[1], "unload") {
				return slashSkillCmd{ok: true, usage: true}
			}
			// /skill <id>
			return slashSkillCmd{ok: true, name: parts[1]}
		default:
			if strings.EqualFold(parts[1], "unload") {
				return slashSkillCmd{ok: true, unload: true, name: parts[2]}
			}
			if strings.EqualFold(parts[1], "load") {
				return slashSkillCmd{ok: true, name: parts[2]}
			}
			return slashSkillCmd{ok: true, usage: true}
		}
	}

	// /<id> or /<id> …extra → load first token as skill id
	return slashSkillCmd{ok: true, name: parts[0]}
}
