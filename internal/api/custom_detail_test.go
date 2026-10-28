package api

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type customDetailResponse struct {
	ID           any      `json:"id"`
	QuestionID   any      `json:"questionId"`
	Type         string   `json:"type"`
	Content      string   `json:"content"`
	ContentHTML  string   `json:"contentHtml"`
	Material     string   `json:"material"`
	MaterialHTML string   `json:"materialHtml"`
	Options      []string `json:"options"`
	Answer       string   `json:"answer"`
	AnswerIndex  int64    `json:"answerIndex"`
	Analysis     string   `json:"analysis"`
	Subject      string   `json:"subject"`
	Chapter      string   `json:"chapter"`
	BatchID      int64    `json:"batchId"`
}

func importCustomBatch(t *testing.T, client *http.Client, baseURL string, payload map[string]any) int64 {
	t.Helper()
	res := postJSON(t, client, baseURL+"/api/custom/import", payload)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("import status = %d", res.StatusCode)
	}
	var imported struct {
		ID int64 `json:"id"`
	}
	decodeJSON(t, res, &imported)
	return imported.ID
}

func customQuestionIDs(t *testing.T, client *http.Client, baseURL string, batchID int64) []int64 {
	t.Helper()
	res, err := client.Get(baseURL + "/api/custom/questions?batch_id=" + jsonNumber(batchID))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Questions []struct {
			ID int64 `json:"id"`
		} `json:"questions"`
	}
	decodeJSON(t, res, &out)
	ids := make([]int64, 0, len(out.Questions))
	for _, q := range out.Questions {
		ids = append(ids, q.ID)
	}
	return ids
}

func TestCustomQuestionDetail(t *testing.T) {
	ts, _ := newTestServer(t)
	client := testClient(t, ts)
	loginTestUser(t, ts, client, "admin", "admin123456")

	batchID := importCustomBatch(t, client, ts.URL, map[string]any{
		"name": "详情批次", "subject": "行测",
		"questions": []map[string]any{{
			"prompt": "看图选择", "material": "材料正文",
			"options": []string{"A. 1", "B. 2"}, "answer": "B", "answer_index": 1,
			"analysis": "因为 B",
			"images": []map[string]string{
				{"role": "stem", "dataUrl": "data:image/png;base64,AAAA"},
				{"role": "material", "dataUrl": "data:image/png;base64,BBBB"},
			},
		}},
	})
	ids := customQuestionIDs(t, client, ts.URL, batchID)
	if len(ids) != 1 {
		t.Fatalf("question count = %d, want 1", len(ids))
	}
	rawID := "custom-" + jsonNumber(ids[0])

	res, err := client.Get(ts.URL + "/api/question?id=" + url.QueryEscape(rawID))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("detail status = %d, want 200", res.StatusCode)
	}
	var q customDetailResponse
	decodeJSON(t, res, &q)

	if q.ID != rawID || q.QuestionID != rawID {
		t.Fatalf("id = %v/%v, want %q", q.ID, q.QuestionID, rawID)
	}
	if q.Type != "custom" {
		t.Fatalf("type = %q, want custom", q.Type)
	}
	if q.Content != "看图选择" || q.Material != "材料正文" {
		t.Fatalf("content/material = %q/%q", q.Content, q.Material)
	}
	if !strings.Contains(q.ContentHTML, "<img") || !strings.Contains(q.MaterialHTML, "<img") {
		t.Fatalf("html missing images: %q / %q", q.ContentHTML, q.MaterialHTML)
	}
	if len(q.Options) != 2 || q.Answer != "B" || q.AnswerIndex != 1 || q.Analysis != "因为 B" {
		t.Fatalf("options/answer/analysis = %+v", q)
	}
	if q.Subject != "行测" || q.Chapter != "详情批次" || q.BatchID != batchID {
		t.Fatalf("subject/chapter/batch = %q/%q/%d", q.Subject, q.Chapter, q.BatchID)
	}
}

func TestCustomQuestionDetailBorrowsGroupMaterial(t *testing.T) {
	ts, _ := newTestServer(t)
	client := testClient(t, ts)
	loginTestUser(t, ts, client, "admin", "admin123456")

	batchID := importCustomBatch(t, client, ts.URL, map[string]any{
		"name": "材料组批次", "subject": "行测",
		"questions": []map[string]any{
			{"prompt": "第一问", "material": "共享材料", "material_id": "g1",
				"options": []string{"A. 1", "B. 2"}, "answer": "A", "answer_index": 0},
			{"prompt": "第二问", "material_id": "g1",
				"options": []string{"A. 1", "B. 2"}, "answer": "A", "answer_index": 0},
		},
	})
	ids := customQuestionIDs(t, client, ts.URL, batchID)
	if len(ids) != 2 {
		t.Fatalf("question count = %d, want 2", len(ids))
	}

	res, err := client.Get(ts.URL + "/api/question?id=custom-" + jsonNumber(ids[1]))
	if err != nil {
		t.Fatal(err)
	}
	var q customDetailResponse
	decodeJSON(t, res, &q)
	if q.Material != "共享材料" {
		t.Fatalf("group member material = %q, want 共享材料", q.Material)
	}
}

func TestCustomQuestionDetailIsolation(t *testing.T) {
	ts, _ := newTestServer(t)
	admin := testClient(t, ts)
	loginTestUser(t, ts, admin, "admin", "admin123456")

	for _, name := range []string{"alice", "bob"} {
		res := postJSON(t, admin, ts.URL+"/api/admin/users", createUserRequest{
			Username: name, Password: name + "123456", Role: "user",
		})
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("create %s status = %d", name, res.StatusCode)
		}
		res.Body.Close()
	}
	alice := testClient(t, ts)
	bob := testClient(t, ts)
	loginTestUser(t, ts, alice, "alice", "alice123456")
	loginTestUser(t, ts, bob, "bob", "bob123456")

	batchID := importCustomBatch(t, alice, ts.URL, map[string]any{
		"name": "alice-batch", "subject": "行测",
		"questions": []map[string]any{{"prompt": "秘密题", "options": []string{"A. 1"}, "answer": "A", "answer_index": 0}},
	})
	ids := customQuestionIDs(t, alice, ts.URL, batchID)
	rawID := "custom-" + jsonNumber(ids[0])

	res, err := bob.Get(ts.URL + "/api/question?id=" + url.QueryEscape(rawID))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("bob detail status = %d, want 404", res.StatusCode)
	}
	res.Body.Close()
}

func TestCustomQuestionInWrongList(t *testing.T) {
	ts, _ := newTestServer(t)
	client := testClient(t, ts)
	loginTestUser(t, ts, client, "admin", "admin123456")

	batchID := importCustomBatch(t, client, ts.URL, map[string]any{
		"name": "错题批次", "subject": "行测",
		"questions": []map[string]any{{"prompt": "自定义错题", "options": []string{"A. 1", "B. 2"}, "answer": "B", "answer_index": 1}},
	})
	ids := customQuestionIDs(t, client, ts.URL, batchID)
	rawID := "custom-" + jsonNumber(ids[0])

	res := postJSON(t, client, ts.URL+"/api/check", map[string]any{
		"questionId": rawID, "selected": []int{0},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("custom check status = %d", res.StatusCode)
	}
	res.Body.Close()
	res = postJSON(t, client, ts.URL+"/api/records", map[string]any{
		"questionId": rawID, "subject": "自定义", "chapter": "错题批次",
		"selected": []int{0},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("custom record status = %d", res.StatusCode)
	}
	res.Body.Close()

	res, err := client.Get(ts.URL + "/api/records/wrong?group=custom")
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		List []struct {
			QuestionID any    `json:"questionId"`
			Content    string `json:"content"`
			Type       any    `json:"type"`
		} `json:"list"`
	}
	decodeJSON(t, res, &out)
	if len(out.List) != 1 {
		t.Fatalf("wrong list length = %d, want 1", len(out.List))
	}
	item := out.List[0]
	if item.QuestionID != rawID {
		t.Fatalf("questionId = %v, want %q", item.QuestionID, rawID)
	}
	if item.Content != "自定义错题" || item.Type != "custom" {
		t.Fatalf("content/type = %q/%v", item.Content, item.Type)
	}
}
