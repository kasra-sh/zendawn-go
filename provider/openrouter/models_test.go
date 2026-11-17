package openrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestIsOpenRouterBase(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://openrouter.ai/api/v1", true},
		{"https://OPENROUTER.AI/api/v1/", true},
		{"https://api.openai.com/v1", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsOpenRouterBase(c.in); got != c.want {
			t.Fatalf("%q: got %v want %v", c.in, got, c.want)
		}
	}
}

func TestListFreeZDRCodingChatModelsPaginate(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing auth")
		}
		q := r.URL.Query()
		if q.Get("zdr") != "true" || q.Get("sort") != "coding-high-to-low" {
			t.Errorf("bad filters: %v", q)
		}
		offset, _ := strconv.Atoi(q.Get("offset"))
		limit, _ := strconv.Atoi(q.Get("limit"))
		all := []Model{
			{ID: "a:free", Name: "A", ContextLength: 100},
			{ID: "b:free", Name: "B", ContextLength: 200},
			{ID: "c:free", Name: "C", ContextLength: 300},
		}
		end := offset + limit
		if end > len(all) {
			end = len(all)
		}
		page := all[offset:end]
		resp := listResponse{Data: page, TotalCount: len(all)}
		if end < len(all) {
			resp.Links = &struct {
				Next string `json:"next"`
			}{Next: "/api/v1/models?offset=" + strconv.Itoa(end)}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	// Force small pages by temporarily swapping pageLimit via direct listModels call.
	ctx := context.Background()
	models, err := listModels(ctx, srv.URL, "test-key", map[string][]string{
		"zdr":              {"true"},
		"sort":             {"coding-high-to-low"},
		"max_price":        {"0"},
		"max_output_price": {"0"},
		"endpoints":        {"/api/v1/chat/completions"},
		"limit":            {"1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 {
		t.Fatalf("got %d models: %+v", len(models), models)
	}
	if models[0].ID != "a:free" || models[2].ID != "c:free" {
		t.Fatalf("order: %+v", models)
	}
	if hits < 3 {
		t.Fatalf("expected paginated hits, got %d", hits)
	}
}

func TestListFreeZDRCodingChatModelsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"nope"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := ListFreeZDRCodingChatModels(context.Background(), srv.URL, "bad")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSpecFromModel(t *testing.T) {
	idx := 42.5
	m := Model{
		ID: "x:free", Name: "X", ContextLength: 8192,
		Pricing: Pricing{Prompt: "0", Completion: "0"},
		Benchmarks: &struct {
			ArtificialAnalysis *struct {
				CodingIndex *float64 `json:"coding_index"`
			} `json:"artificial_analysis"`
		}{
			ArtificialAnalysis: &struct {
				CodingIndex *float64 `json:"coding_index"`
			}{CodingIndex: &idx},
		},
	}
	s := SpecFromModel(m)
	if s.ContextLength != 8192 || s.Name != "X" || s.CodingIndex == nil || *s.CodingIndex != 42.5 {
		t.Fatalf("%+v", s)
	}
}
