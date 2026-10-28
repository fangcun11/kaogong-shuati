package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
