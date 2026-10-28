package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func loginTestUser(t *testing.T, ts *httptest.Server, client *http.Client, username, password string) {
	t.Helper()
	res := postJSON(t, client, ts.URL+"/api/auth/login", credentials{Username: username, Password: password})
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login %s status = %d", username, res.StatusCode)
	}
}

func decodeJSON(t *testing.T, res *http.Response, v any) {
	t.Helper()
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

func TestUserDataIsolation(t *testing.T) {
	ts, _ := newTestServer(t)
	admin := testClient(t, ts)
	loginTestUser(t, ts, admin, "admin", "admin123456")

	// admin creates two ordinary users
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

	// Alice's favorite is invisible to Bob.
	res := postJSON(t, alice, ts.URL+"/api/favorites", map[string]any{
		"questionId": 12345, "subject": "公务员·行测", "chapter": "测试",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("alice favorite status = %d", res.StatusCode)
	}
	res.Body.Close()

	res, err := bob.Get(ts.URL + "/api/favorites")
	if err != nil {
		t.Fatal(err)
	}
	var bobFavorites struct {
		Total int `json:"total"`
	}
	decodeJSON(t, res, &bobFavorites)
	if bobFavorites.Total != 0 {
		t.Fatalf("bob sees %d favorites, want 0", bobFavorites.Total)
	}

	// Alice's custom batch cannot be read or practiced by Bob.
	res = postJSON(t, alice, ts.URL+"/api/custom/import", map[string]any{
		"name": "alice-batch", "subject": "行测",
		"questions": []map[string]any{{"prompt": "1+1", "options": []string{"A. 1", "B. 2"}, "answer": "B", "answer_index": 1}},
	})
	var imported struct {
		ID int64 `json:"id"`
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("alice import status = %d", res.StatusCode)
	}
	decodeJSON(t, res, &imported)

	for _, path := range []string{
		ts.URL + "/api/custom/questions?batch_id=" + jsonNumber(imported.ID),
		ts.URL + "/api/custom/practice?batch_id=" + jsonNumber(imported.ID),
	} {
		res, err := bob.Get(path)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("bob GET %s status = %d, want 404", path, res.StatusCode)
		}
		res.Body.Close()
	}
}

func TestRecordAutoArchivesWrongAfterThreeCorrectDays(t *testing.T) {
	ts, srv := newTestServer(t)
	client := testClient(t, ts)
	loginTestUser(t, ts, client, "admin", "admin123456")
	for _, day := range []string{"-3 day", "-2 day", "-1 day"} {
		if _, err := srv.db.Exec(`INSERT INTO practice_records
			(user_id, question_id, subject, chapter, question_type, selected, is_correct, group_key, created_at)
			VALUES (1, '9999', '公务员·行测', '资料分析', 1, '[]', 1, '行测', datetime('now', ?))`, day); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := srv.db.Exec(`INSERT INTO practice_records
		(user_id, question_id, subject, chapter, question_type, selected, is_correct, group_key)
		VALUES (1, '9999', '公务员·行测', '资料分析', 1, '[]', 0, '行测')`); err != nil {
		t.Fatal(err)
	}
	res := postJSON(t, client, ts.URL+"/api/records", map[string]any{
		"questionId": 9999, "subject": "公务员·行测", "chapter": "资料分析",
		"type": 1, "selected": []int{}, "correct": true,
	})
	var out struct {
		RemovedFromWrong bool `json:"removedFromWrong"`
	}
	decodeJSON(t, res, &out)
	if !out.RemovedFromWrong {
		t.Fatal("wrong record was not auto-archived")
	}
}

func TestWrongListGroupQuery(t *testing.T) {
	ts, srv := newTestServer(t)
	client := testClient(t, ts)
	loginTestUser(t, ts, client, "admin", "admin123456")
	if _, err := srv.db.Exec(`INSERT INTO practice_records
		(user_id, question_id, subject, chapter, question_type, selected, is_correct, group_key)
		VALUES (1, '1001', '公务员·行测', '资料分析', 1, '[]', 0, '行测')`); err != nil {
		t.Fatal(err)
	}
	res, err := client.Get(ts.URL + "/api/records/wrong?group=" + url.QueryEscape("行测"))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Total int   `json:"total"`
		List  []any `json:"list"`
	}
	decodeJSON(t, res, &out)
	if out.Total != 1 || len(out.List) != 1 {
		t.Fatalf("wrong group total=%d list=%d, want 1/1", out.Total, len(out.List))
	}
}

func TestCustomPracticeKeepsGroupWhenCountCutsInside(t *testing.T) {
	ts, _ := newTestServer(t)
	client := testClient(t, ts)
	loginTestUser(t, ts, client, "admin", "admin123456")
	res := postJSON(t, client, ts.URL+"/api/custom/import", map[string]any{
		"name": "材料组", "subject": "行测",
		"questions": []map[string]any{
			{"prompt": "1", "material": "同一材料", "material_id": "group-1", "options": []string{"A. 1", "B. 2"}, "answer": "A", "answer_index": 0},
			{"prompt": "2", "material": "同一材料", "material_id": "group-1", "options": []string{"A. 1", "B. 2"}, "answer": "A", "answer_index": 0},
		},
	})
	var imported struct {
		ID int64 `json:"id"`
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("import status = %d", res.StatusCode)
	}
	decodeJSON(t, res, &imported)
	res, err := client.Get(ts.URL + "/api/custom/practice?batch_id=" + jsonNumber(imported.ID) + "&count=1")
	if err != nil {
		t.Fatal(err)
	}
	var practice struct {
		Questions []any `json:"questions"`
	}
	decodeJSON(t, res, &practice)
	if len(practice.Questions) != 2 {
		t.Fatalf("practice length = %d, want 2 (whole material group)", len(practice.Questions))
	}
}

func TestCustomQuestionImageRendering(t *testing.T) {
	ts, _ := newTestServer(t)
	client := testClient(t, ts)
	loginTestUser(t, ts, client, "admin", "admin123456")

	res := postJSON(t, client, ts.URL+"/api/custom/import", map[string]any{
		"name": "图片题", "subject": "行测",
		"questions": []map[string]any{{
			"prompt": "看图选择", "options": []string{"A. 1", "B. 2"}, "answer": "A", "answer_index": 0,
			"images": []map[string]string{{"role": "stem", "dataUrl": "data:image/png;base64,AAAA"}},
		}},
	})
	var imported struct {
		ID int64 `json:"id"`
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("import status = %d", res.StatusCode)
	}
	decodeJSON(t, res, &imported)

	res, err := client.Get(ts.URL + "/api/custom/practice?batch_id=" + jsonNumber(imported.ID))
	if err != nil {
		t.Fatal(err)
	}
	var practice struct {
		Questions []struct {
			ContentHTML string `json:"contentHtml"`
		} `json:"questions"`
	}
	decodeJSON(t, res, &practice)
	if len(practice.Questions) != 1 || !strings.Contains(practice.Questions[0].ContentHTML, "<img") {
		t.Fatalf("contentHtml does not contain image: %+v", practice.Questions)
	}
}

func jsonNumber(n int64) string {
	return strconv.FormatInt(n, 10)
}
