package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/kasra-sh/zendawn-go/planfile"
	"github.com/kasra-sh/zendawn-go/session"
)

const maxStoredPlans = 5

func sessionHasExistingPlan(s *session.Session) bool {
	if strings.TrimSpace(s.PlanMarkdown) != "" || len(s.PlanTodos) > 0 {
		return true
	}
	if s.CurrentPlanID != "" || len(s.Plans) > 0 {
		return true
	}
	return false
}

func appendPlanContext(b *strings.Builder, s *session.Session, mode string) {
	s.Lock()
	plans := append([]session.StoredPlan{}, s.Plans...)
	curID := s.CurrentPlanID
	body := s.PlanMarkdown
	todos := append([]session.PlanTodo{}, s.PlanTodos...)
	s.Unlock()

	if len(plans) > 0 {
		b.WriteString("\n\n[session plans]\n")
		start := 0
		if len(plans) > maxStoredPlans {
			start = len(plans) - maxStoredPlans
		}
		for _, p := range plans[start:] {
			done, total := 0, len(p.Todos)
			for _, t := range p.Todos {
				if t.Done {
					done++
				}
			}
			mark := ""
			if p.ID == curID {
				mark = " (current)"
			}
			b.WriteString(fmt.Sprintf("- %s — %s — todos %d/%d%s\n", p.ID, p.Title, done, total, mark))
		}
	}

	if strings.TrimSpace(body) != "" || len(todos) > 0 {
		b.WriteString("\n\n[current plan]\n")
		if strings.TrimSpace(body) != "" {
			b.WriteString(body)
			if !strings.HasSuffix(body, "\n") {
				b.WriteByte('\n')
			}
		}
		b.WriteString("\n[todos]\n")
		if len(todos) == 0 {
			b.WriteString("(none)\n")
		} else {
			for _, t := range todos {
				mark := " "
				if t.Done {
					mark = "x"
				}
				b.WriteString(fmt.Sprintf("- [%s] id: %s — %s\n", mark, t.ID, t.Content))
			}
		}
	}

	if mode == session.ModeAgent && len(todos) > 0 {
		b.WriteString("\n\n[plan execution]\nWork only on the first incomplete todo unless the user steers otherwise. When finished, call tool todo with action=complete and that id. Use todo next/list as needed.\n")
		for _, t := range todos {
			if !t.Done {
				b.WriteString(fmt.Sprintf("Active todo: id=%s — %s\n", t.ID, t.Content))
				break
			}
		}
	}
}

// abandonPlanBuffer clears the working plan without committing or archiving.
func abandonPlanBuffer(s *session.Session) {
	s.Lock()
	defer s.Unlock()
	s.PlanMarkdown = ""
	s.PlanTodos = nil
	s.CurrentPlanID = ""
}

// planBufferHasTodos reports whether the working buffer includes any todos.
func planBufferHasTodos(s *session.Session) bool {
	s.Lock()
	raw := s.PlanMarkdown
	n := len(s.PlanTodos)
	s.Unlock()
	if n > 0 {
		return true
	}
	_, todos, err := planfile.Parse(raw)
	if err != nil {
		return false
	}
	return planfile.HasTodos(todos)
}

func commitPlanFromBuffer(s *session.Session, sessionID string) error {
	s.Lock()
	raw := s.PlanMarkdown
	s.Unlock()

	md, todos, err := planfile.Parse(raw)
	if err != nil {
		return err
	}
	if !planfile.HasTodos(todos) {
		return fmt.Errorf("plan must end with todo items")
	}
	normalized := planfile.Normalize(md, todos)
	title := planTitle(md)
	id := fmt.Sprintf("%s-%d", sessionID, time.Now().UTC().Unix())

	stTodos := toSessionTodos(todos)
	stored := session.StoredPlan{
		ID:      id,
		Title:   title,
		Body:    normalized,
		Todos:   stTodos,
		Updated: time.Now().UTC(),
	}

	s.Lock()
	s.PlanMarkdown = normalized
	s.PlanTodos = stTodos
	s.CurrentPlanID = id
	s.Plans = append(s.Plans, stored)
	if len(s.Plans) > maxStoredPlans {
		s.Plans = s.Plans[len(s.Plans)-maxStoredPlans:]
	}
	s.Unlock()
	return nil
}

func planTitle(md string) string {
	for _, line := range strings.Split(md, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return "plan"
}

func toSessionTodos(in []planfile.Todo) []session.PlanTodo {
	out := make([]session.PlanTodo, len(in))
	for i, t := range in {
		out[i] = session.PlanTodo{ID: t.ID, Content: t.Content, Done: t.Done}
	}
	return out
}

func toPlanfileTodos(in []session.PlanTodo) []planfile.Todo {
	out := make([]planfile.Todo, len(in))
	for i, t := range in {
		out[i] = planfile.Todo{ID: t.ID, Content: t.Content, Done: t.Done}
	}
	return out
}

func rewriteCurrentPlanFile(s *session.Session) string {
	s.Lock()
	defer s.Unlock()
	body := planfile.Normalize(stripTodosSection(s.PlanMarkdown), toPlanfileTodos(s.PlanTodos))
	s.PlanMarkdown = body
	if s.CurrentPlanID != "" {
		for i := range s.Plans {
			if s.Plans[i].ID == s.CurrentPlanID {
				s.Plans[i].Body = body
				s.Plans[i].Todos = append([]session.PlanTodo{}, s.PlanTodos...)
				s.Plans[i].Updated = time.Now().UTC()
				break
			}
		}
	}
	return body
}

func stripTodosSection(md string) string {
	rest, _, _ := planfile.Parse(md)
	return rest
}

func archiveAndClearPlan(s *session.Session) {
	s.Lock()
	defer s.Unlock()
	if strings.TrimSpace(s.PlanMarkdown) != "" || len(s.PlanTodos) > 0 {
		id := s.CurrentPlanID
		if id == "" {
			id = fmt.Sprintf("archive-%d", time.Now().UTC().UnixNano())
		}
		s.Plans = append(s.Plans, session.StoredPlan{
			ID:      id,
			Title:   planTitle(s.PlanMarkdown),
			Body:    s.PlanMarkdown,
			Todos:   append([]session.PlanTodo{}, s.PlanTodos...),
			Updated: time.Now().UTC(),
		})
		if len(s.Plans) > maxStoredPlans {
			s.Plans = s.Plans[len(s.Plans)-maxStoredPlans:]
		}
	}
	s.PlanMarkdown = ""
	s.PlanTodos = nil
	s.CurrentPlanID = ""
}
