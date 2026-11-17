package main

import "testing"

func TestParseSlashSkill(t *testing.T) {
	c := parseSlashSkill("/demo")
	if !c.ok || c.name != "demo" || c.unload || c.list || c.usage {
		t.Fatalf("/demo: %+v", c)
	}
	c = parseSlashSkill("/plan-for-review")
	if !c.ok || c.name != "plan-for-review" || c.usage {
		t.Fatalf("/plan-for-review: %+v", c)
	}
	c = parseSlashSkill("/skills")
	if !c.ok || !c.list {
		t.Fatalf("/skills: %+v", c)
	}
	c = parseSlashSkill("/skill load demo")
	if !c.ok || c.name != "demo" || c.unload {
		t.Fatalf("/skill load: %+v", c)
	}
	c = parseSlashSkill("/skill unload demo")
	if !c.ok || c.name != "demo" || !c.unload {
		t.Fatalf("/skill unload: %+v", c)
	}
	c = parseSlashSkill("/create-skill x")
	if !c.ok || c.name != "create-skill" || c.usage {
		t.Fatalf("/create-skill x should load create-skill: %+v", c)
	}
	c = parseSlashSkill("/foo bar")
	if !c.ok || c.name != "foo" {
		t.Fatalf("/foo bar should load foo: %+v", c)
	}
	c = parseSlashSkill("/skill")
	if !c.ok || !c.usage {
		t.Fatalf("/skill incomplete: %+v", c)
	}
	c = parseSlashSkill("/skill load")
	if !c.ok || !c.usage {
		t.Fatalf("/skill load incomplete: %+v", c)
	}
	c = parseSlashSkill("/")
	if !c.ok || !c.usage {
		t.Fatalf("bare /: %+v", c)
	}
	c = parseSlashSkill("hello")
	if c.ok {
		t.Fatalf("expected not slash")
	}
}
