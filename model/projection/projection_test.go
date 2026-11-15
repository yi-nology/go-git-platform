package projection

import (
	"encoding/json"
	"reflect"
	"testing"
)

type head struct {
	Ref    string `json:"ref"`
	Sha    string `json:"sha"`
	Repo   any    `json:"repo,omitempty"`
	Author *struct {
		Login string `json:"login"`
	} `json:"author,omitempty"`
}

type cr struct {
	Number string `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Head   head   `json:"head"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func TestProject(t *testing.T) {
	v := cr{
		Number: "42", Title: "fix: things", State: "open",
		Head: head{Ref: "feature/x", Sha: "abc123"},
		Labels: []struct {
			Name string `json:"name"`
		}{{Name: "bug"}, {Name: "p1"}},
	}

	tests := []struct {
		name    string
		fields  []string
		want    string // JSON of the projection
		wantErr bool
	}{
		{
			name:   "top-level fields",
			fields: []string{"number", "state"},
			want:   `{"number":"42","state":"open"}`,
		},
		{
			name:   "nested path",
			fields: []string{"number", "head.ref"},
			want:   `{"head":{"ref":"feature/x"},"number":"42"}`,
		},
		{
			name:   "array traversed element-wise",
			fields: []string{"labels.name"},
			want:   `{"labels":[{"name":"bug"},{"name":"p1"}]}`,
		},
		{
			name:   "absent field omitted",
			fields: []string{"number", "merged_at"},
			want:   `{"number":"42"}`,
		},
		{
			name:   "wildcard returns full document",
			fields: []string{"*"},
			want:   `{"head":{"ref":"feature/x","sha":"abc123"},"labels":[{"name":"bug"},{"name":"p1"}],"number":"42","state":"open","title":"fix: things"}`,
		},
		{
			name:    "empty field list is an error",
			wantErr: true,
		},
		{
			name:    "invalid path is an error",
			fields:  []string{".head"},
			wantErr: true,
		},
		{
			name:    "segment-level wildcard is an error",
			fields:  []string{"labels.*"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Project(v, tt.fields...)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Project() err = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Project() err = %v", err)
			}
			raw, _ := json.Marshal(got)
			if string(raw) != tt.want {
				t.Fatalf("Project() = %s, want %s", raw, tt.want)
			}
		})
	}
}

func TestProjectPreservesNumbers(t *testing.T) {
	v := map[string]any{"id": 12345, "ratio": 0.5}
	got, err := Project(v, "id", "ratio")
	if err != nil {
		t.Fatalf("Project() err = %v", err)
	}
	if id := got["id"].(json.Number).String(); id != "12345" {
		t.Fatalf("id = %v, want json.Number 12345", id)
	}
}

func TestProjectList(t *testing.T) {
	items := []cr{{Number: "1", Title: "a"}, {Number: "2", Title: "b"}}
	got, err := ProjectList(items, "number")
	if err != nil {
		t.Fatalf("ProjectList() err = %v", err)
	}
	if len(got) != 2 || got[0]["number"] != "1" || got[1]["number"] != "2" {
		t.Fatalf("ProjectList() = %v", got)
	}
}

// issue #53：字段集合并与列出顺序无关——祖先路径排在后代之后必须吸收后代
// （旧实现把窄路径建成的嵌套 keepTree 原样返回，宽路径被静默忽略）。
func TestProjectFieldUnionOrderIndependent(t *testing.T) {
	doc := map[string]any{
		"head": map[string]any{"ref": "main", "sha": "abc123", "repo": "x"},
		"num":  float64(7),
	}
	wide := map[string]any{"head": map[string]any{"ref": "main", "sha": "abc123", "repo": "x"}}

	t.Run("ancestor_after_descendant_upgrades_to_wide", func(t *testing.T) {
		got, err := Project(doc, "head.ref", "head")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		if !reflect.DeepEqual(got, wide) {
			t.Fatalf("Project(head.ref, head) = %v, want %v (ancestor path must absorb the descendant)", got, wide)
		}
	})

	t.Run("permutations_agree", func(t *testing.T) {
		a, err := Project(doc, "head.ref", "head")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		b, err := Project(doc, "head", "head.ref")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		c, err := Project(doc, "head")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(b, c) {
			t.Fatalf("同一字段集合的次序置换投影结果不一致:\nnarrow→wide = %v\nwide→narrow = %v\nwide alone = %v", a, b, c)
		}
	})

	t.Run("deep_ancestor_after_deep_descendant", func(t *testing.T) {
		d := map[string]any{"a": map[string]any{"b": map[string]any{"c": 1, "d": 2}}}
		got, err := Project(d, "a.b.c", "a")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		// Project 经 UseNumber 解码：数值以 json.Number 呈现。
		want := map[string]any{"a": map[string]any{"b": map[string]any{"c": json.Number("1"), "d": json.Number("2")}}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Project(a.b.c, a) = %v, want %v", got, want)
		}
	})

	t.Run("descendant_after_ancestor_stays_wide", func(t *testing.T) {
		got, err := Project(doc, "head", "head.ref")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		if !reflect.DeepEqual(got, wide) {
			t.Fatalf("Project(head, head.ref) = %v, want %v", got, wide)
		}
	})

	t.Run("unrelated_fields_untouched", func(t *testing.T) {
		got, err := Project(doc, "num", "head.sha", "head")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		want := map[string]any{"num": json.Number("7"), "head": map[string]any{"ref": "main", "sha": "abc123", "repo": "x"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Project(num, head.sha, head) = %v, want %v", got, want)
		}
	})
}
